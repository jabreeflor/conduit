package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// Client connects to one external MCP server and exposes its tools.
type Client struct {
	entry     ServerEntry
	transport Transport
	proc      *exec.Cmd // non-nil for stdio servers

	nextID atomic.Int64
	mu     sync.Mutex
	// pending maps request ID → response channel
	pending map[string]chan Message

	tools []ToolDef // cached after ListTools
}

// Connect dials the MCP server described by entry.
// For stdio servers it spawns the subprocess. For HTTP servers it performs a
// stateless HTTP handshake on first use.
func Connect(ctx context.Context, entry ServerEntry) (*Client, error) {
	c := &Client{
		entry:   entry,
		pending: make(map[string]chan Message),
	}

	switch entry.Transport {
	case TransportStdio:
		if entry.Command == "" {
			return nil, fmt.Errorf("mcp: stdio server %q requires command", entry.Name)
		}
		cmd := exec.CommandContext(ctx, entry.Command, entry.Args...)
		cmd.Env = append(cmd.Environ(), entry.Env...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("mcp: stdin pipe for %q: %w", entry.Name, err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("mcp: stdout pipe for %q: %w", entry.Name, err)
		}
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("mcp: start %q: %w", entry.Name, err)
		}
		c.proc = cmd
		c.transport = NewStdioTransport(stdout, stdin)
		go c.readLoop(context.Background())

	case TransportStreamingHTTP:
		if entry.URL == "" {
			return nil, fmt.Errorf("mcp: http server %q requires url", entry.Name)
		}
		// HTTP transport is stateless per-request; no persistent connection needed.
		c.transport = nil

	default:
		return nil, fmt.Errorf("mcp: unknown transport %q for server %q", entry.Transport, entry.Name)
	}

	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// Close shuts down the client's transport and, for stdio servers, waits for
// the subprocess to exit.
func (c *Client) Close() error {
	if c.transport != nil {
		_ = c.transport.Close()
	}
	if c.proc != nil {
		return c.proc.Wait()
	}
	return nil
}

// ListTools returns the tool list advertised by this server, filtered by the
// server entry's allowlist. Results are cached after the first call.
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
	if c.tools != nil {
		return c.tools, nil
	}

	resp, err := c.call(ctx, MethodToolsList, nil)
	if err != nil {
		return nil, err
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/list response: %w", err)
	}

	tools := result.Tools
	if len(c.entry.Allowlist) > 0 {
		allowed := make(map[string]bool, len(c.entry.Allowlist))
		for _, name := range c.entry.Allowlist {
			allowed[name] = true
		}
		filtered := tools[:0]
		for _, t := range tools {
			if allowed[t.Name] {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}

	c.tools = tools
	return c.tools, nil
}

// CallTool invokes a tool on the remote server and returns the content.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) ([]Content, error) {
	params := ToolCallParams{Name: name, Arguments: args}
	resp, err := c.call(ctx, MethodToolsCall, params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/call response: %w", err)
	}
	if result.IsError {
		texts := make([]string, 0, len(result.Content))
		for _, c := range result.Content {
			if c.Type == ContentTypeText {
				texts = append(texts, c.Text)
			}
		}
		return nil, fmt.Errorf("mcp tool error: %s", strings.Join(texts, "; "))
	}
	return result.Content, nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := InitializeParams{
		ProtocolVersion: Version,
		ClientInfo:      Info{Name: "conduit", Version: "dev"},
	}
	resp, err := c.call(ctx, MethodInitialize, params)
	if err != nil {
		return fmt.Errorf("mcp: initialize %q: %w", c.entry.Name, err)
	}
	if resp.Error != nil {
		return fmt.Errorf("mcp: initialize %q: %w", c.entry.Name, resp.Error)
	}
	// Send the initialized notification (fire-and-forget, no ID).
	notif := Message{
		JSONRPC: JSONRPCVersion,
		Method:  MethodInitialized,
	}
	return c.transport.Send(ctx, notif)
}

func (c *Client) call(ctx context.Context, method string, params any) (Message, error) {
	if c.entry.Transport == TransportStreamingHTTP {
		return c.httpCall(ctx, method, params)
	}
	return c.stdioCall(ctx, method, params)
}

func (c *Client) stdioCall(ctx context.Context, method string, params any) (Message, error) {
	id := c.nextID.Add(1)
	idJSON, _ := json.Marshal(id)
	idRaw := json.RawMessage(idJSON)

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return Message{}, err
		}
	}

	msg := Message{
		JSONRPC: JSONRPCVersion,
		ID:      &idRaw,
		Method:  method,
		Params:  rawParams,
	}

	ch := make(chan Message, 1)
	key := fmt.Sprintf("%d", id)
	c.mu.Lock()
	c.pending[key] = ch
	c.mu.Unlock()

	if err := c.transport.Send(ctx, msg); err != nil {
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
		return Message{}, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, key)
		c.mu.Unlock()
		return Message{}, ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
}

// readLoop drains the transport and fans responses out to waiting callers.
func (c *Client) readLoop(ctx context.Context) {
	for {
		msg, err := c.transport.Receive(ctx)
		if err != nil {
			return
		}
		if msg.ID == nil {
			continue // notification — ignore for now
		}
		var key string
		var raw int64
		if err := json.Unmarshal(*msg.ID, &raw); err == nil {
			key = fmt.Sprintf("%d", raw)
		}
		c.mu.Lock()
		ch, ok := c.pending[key]
		if ok {
			delete(c.pending, key)
		}
		c.mu.Unlock()
		if ok {
			ch <- msg
		}
	}
}

func (c *Client) httpCall(ctx context.Context, method string, params any) (Message, error) {
	id := c.nextID.Add(1)
	idJSON, _ := json.Marshal(id)
	idRaw := json.RawMessage(idJSON)

	var rawParams json.RawMessage
	if params != nil {
		var err error
		rawParams, err = json.Marshal(params)
		if err != nil {
			return Message{}, err
		}
	}

	req := Message{
		JSONRPC: JSONRPCVersion,
		ID:      &idRaw,
		Method:  method,
		Params:  rawParams,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.entry.URL, strings.NewReader(string(body)))
	if err != nil {
		return Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return Message{}, fmt.Errorf("mcp: http call to %q: %w", c.entry.URL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, fmt.Errorf("mcp: read response from %q: %w", c.entry.URL, err)
	}

	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Message{}, fmt.Errorf("mcp: parse response from %q: %w", c.entry.URL, err)
	}
	return msg, nil
}

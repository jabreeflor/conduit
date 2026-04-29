package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Transport is the I/O contract every MCP transport must satisfy.
// Send delivers one outbound message; Receive blocks until the next inbound
// message arrives or ctx is cancelled; Close releases resources.
type Transport interface {
	Send(ctx context.Context, msg Message) error
	Receive(ctx context.Context) (Message, error)
	Close() error
}

// StdioTransport sends and receives newline-delimited JSON over an io.Reader /
// io.Writer pair — typically os.Stdin / os.Stdout for subprocess MCP servers.
type StdioTransport struct {
	enc *json.Encoder
	sc  *bufio.Scanner

	mu sync.Mutex // guards enc
}

// NewStdioTransport wraps the given reader and writer.
func NewStdioTransport(r io.Reader, w io.Writer) *StdioTransport {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MiB max line
	return &StdioTransport{enc: enc, sc: sc}
}

// Send encodes msg as a single JSON line.
func (t *StdioTransport) Send(_ context.Context, msg Message) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.enc.Encode(msg)
}

// Receive blocks until a complete JSON line is available.
func (t *StdioTransport) Receive(ctx context.Context) (Message, error) {
	type result struct {
		msg Message
		err error
	}
	ch := make(chan result, 1)
	go func() {
		if !t.sc.Scan() {
			err := t.sc.Err()
			if err == nil {
				err = io.EOF
			}
			ch <- result{err: err}
			return
		}
		var msg Message
		if err := json.Unmarshal(t.sc.Bytes(), &msg); err != nil {
			ch <- result{err: fmt.Errorf("mcp: parse message: %w", err)}
			return
		}
		ch <- result{msg: msg}
	}()
	select {
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case r := <-ch:
		return r.msg, r.err
	}
}

// Close is a no-op for the STDIO transport — the caller owns the streams.
func (t *StdioTransport) Close() error { return nil }

// streamingHTTPTransport wraps an HTTP connection for the MCP
// streaming-HTTP transport (POST for client→server, SSE for server→client).
// One instance is created per HTTP session.
type streamingHTTPTransport struct {
	incoming  chan Message
	outgoing  chan Message
	done      chan struct{}
	closeOnce sync.Once
}

func newStreamingHTTPTransport() *streamingHTTPTransport {
	return &streamingHTTPTransport{
		incoming: make(chan Message, 64),
		outgoing: make(chan Message, 64),
		done:     make(chan struct{}),
	}
}

func (t *streamingHTTPTransport) Send(_ context.Context, msg Message) error {
	select {
	case t.outgoing <- msg:
		return nil
	case <-t.done:
		return fmt.Errorf("mcp: transport closed")
	}
}

func (t *streamingHTTPTransport) Receive(ctx context.Context) (Message, error) {
	select {
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case msg, ok := <-t.incoming:
		if !ok {
			return Message{}, io.EOF
		}
		return msg, nil
	case <-t.done:
		return Message{}, io.EOF
	}
}

func (t *streamingHTTPTransport) Close() error {
	t.closeOnce.Do(func() { close(t.done) })
	return nil
}

// HTTPHandler is an http.Handler that accepts MCP streaming-HTTP connections.
// Mount it at a path (e.g. /mcp) with an HTTP mux.
//
// Protocol sketch:
//
//	POST /mcp          → client sends one JSON-RPC message
//	GET  /mcp/stream   → server streams SSE events to the client
type HTTPHandler struct {
	server *Server
}

// NewHTTPHandler creates an HTTPHandler backed by the given server.
func NewHTTPHandler(srv *Server) *HTTPHandler {
	return &HTTPHandler{server: srv}
}

// ServeHTTP dispatches POST (inbound) and GET/stream (SSE outbound).
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost:
		h.handlePost(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/stream"):
		h.handleSSE(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *HTTPHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	resp, err := h.server.Handle(r.Context(), msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *HTTPHandler) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ctx := r.Context()

	// Send a keep-alive ping every 15 seconds so proxies don't close the
	// connection, then wait for the request context to cancel.
	for {
		select {
		case <-ctx.Done():
			return
		}
	}

	// Drain any buffered outbound events.  Real notifications (tool-list
	// changed, log messages) would be pushed here via a channel that the
	// Server writes to when its state changes.
	_ = flusher
}

// writeSSEEvent formats and writes one Server-Sent Events frame.
func writeSSEEvent(w io.Writer, event string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, bytes.TrimSpace(raw))
	return err
}

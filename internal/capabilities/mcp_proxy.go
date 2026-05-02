package capabilities

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// mcpProxy is shared scaffolding for capabilities whose tools live in an
// external MCP server (Browser via Chrome MCP, Desktop via
// open-codex-computer-use). The proxy:
//
//   - Looks up the configured MCP server name on Init via MCPClientFactory.
//   - Caches the client + tool list for the lifetime of the adapter.
//   - Reports "capability unavailable" cleanly when the server is missing,
//     instead of crashing the harness.
//   - Prefixes every tool name with the capability kind so a Chrome MCP tool
//     like "navigate" surfaces as "browser.navigate" — preserving uniqueness
//     across capabilities.
//   - Strips the prefix on Dispatch and forwards the raw tool name to MCP.
type mcpProxy struct {
	kind     Kind
	server   string
	approval Approval
	factory  MCPClientFactory

	mu      sync.Mutex
	client  MCPToolClient
	tools   []Tool
	unavail string // when set, every method returns UnavailableError(reason)
}

// newMCPProxy builds a proxy. server is the configured MCP server name; an
// empty server triggers immediate unavailability.
func newMCPProxy(kind Kind, server string, approval Approval, factory MCPClientFactory) *mcpProxy {
	if approval == nil {
		approval = AllowAllApproval
	}
	return &mcpProxy{kind: kind, server: server, approval: approval, factory: factory}
}

func (p *mcpProxy) Kind() Kind { return p.kind }

// Init resolves the underlying MCP client. A missing factory or unknown
// server name marks the adapter unavailable but is not a fatal error — the
// rest of the harness keeps running and Dispatch reports the reason.
func (p *mcpProxy) Init(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.factory == nil {
		p.unavail = fmt.Sprintf("no MCP client factory configured for %s capability", p.kind)
		return nil
	}
	if p.server == "" {
		p.unavail = fmt.Sprintf("no MCP server configured for %s capability", p.kind)
		return nil
	}

	client, err := p.factory(ctx, p.server)
	if err != nil {
		p.unavail = fmt.Sprintf("connect to MCP server %q: %v", p.server, err)
		return nil
	}
	if client == nil {
		p.unavail = fmt.Sprintf("MCP server %q is not registered", p.server)
		return nil
	}
	p.client = client

	// Pre-fetch tools so ListTools is cheap and so connection problems
	// surface immediately during Init.
	defs, err := client.ListTools(ctx)
	if err != nil {
		p.unavail = fmt.Sprintf("list tools on %q: %v", p.server, err)
		_ = client.Close()
		p.client = nil
		return nil
	}
	prefix := string(p.kind) + "."
	tools := make([]Tool, 0, len(defs))
	for _, d := range defs {
		tools = append(tools, Tool{
			Capability:  p.kind,
			Name:        prefix + d.Name,
			Description: d.Description,
			Schema:      d.InputSchema,
		})
	}
	p.tools = tools
	return nil
}

// ListTools returns the cached tool list.
func (p *mcpProxy) ListTools(_ context.Context) ([]Tool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unavail != "" {
		return nil, newUnavailable(p.kind, p.unavail)
	}
	return append([]Tool(nil), p.tools...), nil
}

// Dispatch consults the approval gate and forwards the call to the MCP server.
func (p *mcpProxy) Dispatch(ctx context.Context, name string, args map[string]any) (Result, error) {
	p.mu.Lock()
	if p.unavail != "" {
		reason := p.unavail
		p.mu.Unlock()
		return Result{}, newUnavailable(p.kind, reason)
	}
	client := p.client
	p.mu.Unlock()

	prefix := string(p.kind) + "."
	if !strings.HasPrefix(name, prefix) {
		return Result{}, fmt.Errorf("%s: tool %q is not owned by this capability", p.kind, name)
	}
	rawName := strings.TrimPrefix(name, prefix)

	approved, err := p.approval.RequireApproval(ctx, string(p.kind), rawName)
	if err != nil {
		return Result{}, fmt.Errorf("%s: approval check failed: %w", p.kind, err)
	}
	if !approved {
		return Result{Text: fmt.Sprintf("%s tool %q denied by user", p.kind, rawName), IsError: true}, nil
	}

	content, err := client.CallTool(ctx, rawName, args)
	if err != nil {
		return Result{Text: err.Error(), IsError: true}, nil
	}

	var b strings.Builder
	for i, c := range content {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch c.Type {
		case "text", "":
			b.WriteString(c.Text)
		case "image":
			fmt.Fprintf(&b, "[image %s, %d bytes]", c.MIME, len(c.Data))
		default:
			fmt.Fprintf(&b, "[%s content]", c.Type)
		}
	}
	return Result{Text: b.String()}, nil
}

// Shutdown closes the underlying MCP client if one was opened.
func (p *mcpProxy) Shutdown(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return nil
	}
	err := p.client.Close()
	p.client = nil
	p.tools = nil
	return err
}

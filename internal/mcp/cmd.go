package mcp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

// RunCLI is the entry point for the `conduit mcp` subcommand.
// args are the arguments after "mcp" (e.g. ["list"], ["serve"], ["call", ...]).
func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runList(ctx, stdout, stderr)
	}
	switch args[0] {
	case "list":
		return runList(ctx, stdout, stderr)
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
	case "call":
		return runCall(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown mcp subcommand %q — try: list, serve, call", args[0])
	}
}

// runList prints all configured MCP servers and their tool counts.
func runList(ctx context.Context, stdout, stderr io.Writer) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("mcp list: load config: %w", err)
	}

	if len(cfg.Servers) == 0 {
		fmt.Fprintln(stdout, "No MCP servers configured.")
		fmt.Fprintln(stdout, "Add servers to ~/.conduit/mcp.yaml or .conduit/mcp.yaml")
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tTRANSPORT\tSTATUS\tTOOLS")

	for _, entry := range cfg.Servers {
		if !entry.IsEnabled() {
			fmt.Fprintf(tw, "%s\t%s\tdisabled\t—\n", entry.Name, entry.Transport)
			continue
		}

		client, err := Connect(ctx, entry)
		if err != nil {
			fmt.Fprintf(tw, "%s\t%s\terror: %s\t—\n", entry.Name, entry.Transport, err)
			continue
		}

		tools, err := client.ListTools(ctx)
		_ = client.Close()
		if err != nil {
			fmt.Fprintf(tw, "%s\t%s\terror: %s\t—\n", entry.Name, entry.Transport, err)
			continue
		}

		toolNames := make([]string, 0, len(tools))
		for _, t := range tools {
			toolNames = append(toolNames, t.Name)
		}
		sort.Strings(toolNames)

		fmt.Fprintf(tw, "%s\t%s\tok\t%s\n",
			entry.Name,
			entry.Transport,
			strings.Join(toolNames, ", "),
		)
	}
	return tw.Flush()
}

// runServe starts Conduit as an MCP server so external clients can call
// Conduit's built-in tools. Address defaults to "127.0.0.1:0".
func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	addr := "127.0.0.1:0"
	if len(args) > 0 {
		addr = args[0]
	}

	srv := NewServer("conduit", "dev")
	registerBuiltins(srv)

	handler := NewHTTPHandler(srv)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mcp serve: listen %s: %w", addr, err)
	}

	fmt.Fprintf(stdout, "conduit MCP server listening on %s\n", ln.Addr())

	httpSrv := &http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Close()
	}()
	return httpSrv.Serve(ln)
}

// runCall invokes a single tool on a named MCP server.
// Usage: conduit mcp call <server> <tool> [key=value ...]
func runCall(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: conduit mcp call <server> <tool> [key=value ...]")
	}
	serverName, toolName := args[0], args[1]

	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("mcp call: %w", err)
	}

	var entry *ServerEntry
	for i := range cfg.Servers {
		if cfg.Servers[i].Name == serverName {
			entry = &cfg.Servers[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("mcp call: server %q not found in config", serverName)
	}

	toolArgs := parseKV(args[2:])
	client, err := Connect(ctx, *entry)
	if err != nil {
		return fmt.Errorf("mcp call: connect to %q: %w", serverName, err)
	}
	defer client.Close()

	gate := NewApprovalGate(stderr, func(ctx context.Context, tool string, a map[string]any) (bool, error) {
		fmt.Fprint(stderr, "[y/N] ")
		var answer string
		fmt.Fscan(os.Stdin, &answer)
		return strings.ToLower(strings.TrimSpace(answer)) == "y", nil
	})

	ok, err := gate.Check(ctx, toolName, toolArgs)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("mcp call: tool %q denied by approval gate", toolName)
	}

	content, err := client.CallTool(ctx, toolName, toolArgs)
	if err != nil {
		return fmt.Errorf("mcp call: %w", err)
	}

	for _, c := range content {
		if c.Type == ContentTypeText {
			fmt.Fprintln(stdout, c.Text)
		}
	}
	return nil
}

// parseKV converts ["key=value", ...] into a map.
func parseKV(pairs []string) map[string]any {
	out := make(map[string]any, len(pairs))
	for _, pair := range pairs {
		k, v, _ := strings.Cut(pair, "=")
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// registerBuiltins adds Conduit's built-in tools to the MCP server so
// external clients can discover and invoke them via the protocol.
func registerBuiltins(srv *Server) {
	srv.RegisterTool(ToolDef{
		Name:        "conduit_ping",
		Description: "Returns pong. Useful for verifying the server is reachable.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(_ context.Context, _ map[string]any) ([]Content, error) {
		return []Content{{Type: ContentTypeText, Text: "pong"}}, nil
	})
}

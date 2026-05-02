package capabilities

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// ---------- shared fakes ----------

type fakeMCPClient struct {
	tools    []MCPToolDef
	listErr  error
	calls    []mcpCall
	closeErr error
	closed   bool
	respond  func(name string, args map[string]any) ([]MCPContent, error)
}

type mcpCall struct {
	name string
	args map[string]any
}

func (f *fakeMCPClient) ListTools(_ context.Context) ([]MCPToolDef, error) {
	return f.tools, f.listErr
}
func (f *fakeMCPClient) CallTool(_ context.Context, name string, args map[string]any) ([]MCPContent, error) {
	f.calls = append(f.calls, mcpCall{name: name, args: args})
	if f.respond != nil {
		return f.respond(name, args)
	}
	return []MCPContent{{Type: "text", Text: "ok:" + name}}, nil
}
func (f *fakeMCPClient) Close() error { f.closed = true; return f.closeErr }

func factoryWith(clients map[string]*fakeMCPClient) MCPClientFactory {
	return func(_ context.Context, name string) (MCPToolClient, error) {
		c, ok := clients[name]
		if !ok {
			return nil, nil // server not registered
		}
		return c, nil
	}
}

// ---------- Shell ----------

func TestShellCapability_DispatchRunsCommand(t *testing.T) {
	approval := ApprovalFunc(func(_ context.Context, app string, action string) (bool, error) {
		if app != "shell" {
			t.Fatalf("approval app = %q, want shell", app)
		}
		if !strings.HasPrefix(action, "echo ") {
			t.Fatalf("approval action = %q, want echo prefix", action)
		}
		return true, nil
	})

	cap := NewShellCapability(Config{Shell: true}, approval).withRunner(
		func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "echo" {
				t.Fatalf("runner name = %q, want echo", name)
			}
			return []byte(strings.Join(args, " ") + "\n"), nil
		},
	)
	if err := cap.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	res, err := cap.Dispatch(context.Background(), "shell.exec", map[string]any{
		"command": "echo",
		"args":    []any{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("dispatch err = %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	if res.Text != "hello world" {
		t.Fatalf("text = %q, want %q", res.Text, "hello world")
	}
}

func TestShellCapability_DenialReturnsErrorResult(t *testing.T) {
	cap := NewShellCapability(Config{Shell: true}, DenyAllApproval).withRunner(
		func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			t.Fatal("runner must not run when approval denied")
			return nil, nil
		},
	)
	if err := cap.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := cap.Dispatch(context.Background(), "shell.exec", map[string]any{"command": "rm"})
	if err != nil {
		t.Fatalf("dispatch should not error on user denial, got %v", err)
	}
	if !res.IsError {
		t.Fatalf("denied call must produce IsError=true, got %#v", res)
	}
}

func TestShellCapability_AllowlistRejectsUnknownCommand(t *testing.T) {
	cfg := Config{Shell: true, ShellAllowedCommands: []string{"git", "ls"}}
	cap := NewShellCapability(cfg, AllowAllApproval).withRunner(
		func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			t.Fatal("runner must not run for disallowed command")
			return nil, nil
		},
	)
	_ = cap.Init(context.Background())
	_, err := cap.Dispatch(context.Background(), "shell.exec", map[string]any{"command": "rm"})
	if err == nil || !strings.Contains(err.Error(), "not in the allowed list") {
		t.Fatalf("expected allow-list rejection, got %v", err)
	}
}

func TestShellCapability_ListToolsExposesShellExec(t *testing.T) {
	cap := NewShellCapability(Config{Shell: true}, AllowAllApproval)
	if _, err := cap.ListTools(context.Background()); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("list before init: err = %v, want unavailable", err)
	}
	_ = cap.Init(context.Background())
	tools, err := cap.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "shell.exec" || tools[0].Capability != KindShell {
		t.Fatalf("tools = %#v", tools)
	}
}

// ---------- Browser / Desktop (MCP-backed) ----------

func TestBrowserCapability_Unavailable_WhenServerMissing(t *testing.T) {
	cap := NewBrowserCapability(Config{Browser: true}, AllowAllApproval, factoryWith(nil))
	if err := cap.Init(context.Background()); err != nil {
		t.Fatalf("init err = %v", err)
	}

	_, err := cap.ListTools(context.Background())
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("ListTools err = %v, want unavailable", err)
	}
	if !strings.Contains(err.Error(), "chrome") {
		t.Fatalf("unavailable reason should mention server name, got %q", err.Error())
	}

	_, err = cap.Dispatch(context.Background(), "browser.navigate", nil)
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("Dispatch err = %v, want unavailable", err)
	}
}

func TestDesktopCapability_Unavailable_WhenFactoryNil(t *testing.T) {
	cap := NewDesktopCapability(Config{Desktop: true}, AllowAllApproval, nil)
	if err := cap.Init(context.Background()); err != nil {
		t.Fatalf("init err = %v", err)
	}
	_, err := cap.Dispatch(context.Background(), "desktop.click", nil)
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
	var ue *UnavailableError
	if !errors.As(err, &ue) || ue.Kind != KindDesktop {
		t.Fatalf("expected UnavailableError{Kind:desktop}, got %v", err)
	}
}

func TestBrowserCapability_ListToolsAndDispatch_ProxyToMCP(t *testing.T) {
	fake := &fakeMCPClient{
		tools: []MCPToolDef{
			{Name: "navigate", Description: "go to url", InputSchema: map[string]any{"type": "object"}},
			{Name: "screenshot", Description: "snap", InputSchema: map[string]any{"type": "object"}},
		},
	}
	clients := map[string]*fakeMCPClient{"chrome": fake}

	cap := NewBrowserCapability(Config{Browser: true}, AllowAllApproval, factoryWith(clients))
	if err := cap.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	tools, err := cap.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, t := range tools {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	want := []string{"browser.navigate", "browser.screenshot"}
	if fmt.Sprintf("%v", names) != fmt.Sprintf("%v", want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}

	res, err := cap.Dispatch(context.Background(), "browser.navigate", map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("dispatch err = %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	if len(fake.calls) != 1 || fake.calls[0].name != "navigate" {
		t.Fatalf("forwarded calls = %#v, want one with raw name 'navigate'", fake.calls)
	}
	if u, _ := fake.calls[0].args["url"].(string); u != "https://example.com" {
		t.Fatalf("args lost in proxy: %#v", fake.calls[0].args)
	}
}

func TestDesktopCapability_DefaultServerName(t *testing.T) {
	fake := &fakeMCPClient{}
	clients := map[string]*fakeMCPClient{"open-codex-computer-use": fake}
	cap := NewDesktopCapability(Config{Desktop: true}, AllowAllApproval, factoryWith(clients))
	if err := cap.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cap.unavail != "" {
		t.Fatalf("expected available, got unavailable=%q", cap.unavail)
	}
}

func TestMCPProxy_DispatchDeniedByApproval(t *testing.T) {
	fake := &fakeMCPClient{tools: []MCPToolDef{{Name: "click"}}}
	cap := NewDesktopCapability(
		Config{Desktop: true},
		DenyAllApproval,
		factoryWith(map[string]*fakeMCPClient{"open-codex-computer-use": fake}),
	)
	_ = cap.Init(context.Background())
	res, err := cap.Dispatch(context.Background(), "desktop.click", map[string]any{"x": 10, "y": 20})
	if err != nil {
		t.Fatalf("err = %v, want clean denial", err)
	}
	if !res.IsError {
		t.Fatalf("denial must surface as IsError, got %#v", res)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("server must not be called when denied; got %#v", fake.calls)
	}
}

// ---------- Manager ----------

func TestManager_OptInOnly(t *testing.T) {
	mgr := NewManager(Config{Shell: false, Browser: false, Desktop: false}, AllowAllApproval, nil)
	if got := mgr.Capabilities(); len(got) != 0 {
		t.Fatalf("nothing enabled, got %v", got)
	}
}

func TestManager_DefaultIsShellOnly(t *testing.T) {
	mgr := NewManager(DefaultConfig(), AllowAllApproval, nil)
	caps := mgr.Capabilities()
	if len(caps) != 1 || caps[0] != KindShell {
		t.Fatalf("default capabilities = %v, want [shell]", caps)
	}
}

func TestManager_RoutesByPrefix(t *testing.T) {
	fakeChrome := &fakeMCPClient{tools: []MCPToolDef{{Name: "navigate"}}}
	fakeDesktop := &fakeMCPClient{tools: []MCPToolDef{{Name: "click"}}}
	clients := map[string]*fakeMCPClient{
		"chrome":                  fakeChrome,
		"open-codex-computer-use": fakeDesktop,
	}

	mgr := NewManager(Config{Shell: true, Browser: true, Desktop: true}, AllowAllApproval, factoryWith(clients))
	// Replace the shell adapter with one that uses a stub runner so we can
	// assert routing without invoking real commands.
	for i, c := range mgr.caps {
		if c.Kind() == KindShell {
			mgr.caps[i] = NewShellCapability(Config{Shell: true}, AllowAllApproval).withRunner(
				func(_ context.Context, name string, args ...string) ([]byte, error) {
					return []byte(name + "/" + strings.Join(args, ",")), nil
				},
			)
		}
	}
	if err := mgr.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer mgr.Shutdown(context.Background())

	res, err := mgr.Dispatch(context.Background(), "shell.exec", map[string]any{"command": "uname", "args": []any{"-a"}})
	if err != nil || res.IsError {
		t.Fatalf("shell dispatch: err=%v res=%#v", err, res)
	}
	if !strings.Contains(res.Text, "uname/-a") {
		t.Fatalf("shell text = %q", res.Text)
	}

	if _, err := mgr.Dispatch(context.Background(), "browser.navigate", map[string]any{"url": "x"}); err != nil {
		t.Fatalf("browser dispatch err = %v", err)
	}
	if len(fakeChrome.calls) != 1 || fakeChrome.calls[0].name != "navigate" {
		t.Fatalf("chrome calls = %#v", fakeChrome.calls)
	}

	if _, err := mgr.Dispatch(context.Background(), "desktop.click", map[string]any{}); err != nil {
		t.Fatalf("desktop dispatch err = %v", err)
	}
	if len(fakeDesktop.calls) != 1 || fakeDesktop.calls[0].name != "click" {
		t.Fatalf("desktop calls = %#v", fakeDesktop.calls)
	}
}

func TestManager_DispatchRejectsUnknownPrefix(t *testing.T) {
	mgr := NewManager(DefaultConfig(), AllowAllApproval, nil)
	_ = mgr.Init(context.Background())
	_, err := mgr.Dispatch(context.Background(), "unknown.tool", nil)
	if err == nil || !strings.Contains(err.Error(), "no capability prefix") && !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("err = %v, want unknown-prefix rejection", err)
	}

	_, err = mgr.Dispatch(context.Background(), "noprefix", nil)
	if err == nil || !strings.Contains(err.Error(), "no capability prefix") {
		t.Fatalf("err = %v, want missing-prefix rejection", err)
	}
}

func TestManager_DispatchDisabledCapabilityFails(t *testing.T) {
	mgr := NewManager(Config{Shell: true}, AllowAllApproval, nil)
	_ = mgr.Init(context.Background())
	_, err := mgr.Dispatch(context.Background(), "browser.navigate", nil)
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("err = %v, want not-enabled", err)
	}
}

func TestManager_ListToolsSkipsUnavailable(t *testing.T) {
	// browser enabled but factory has no chrome client: should be skipped silently.
	mgr := NewManager(Config{Shell: true, Browser: true}, AllowAllApproval, factoryWith(map[string]*fakeMCPClient{}))
	for i, c := range mgr.caps {
		if c.Kind() == KindShell {
			mgr.caps[i] = NewShellCapability(Config{Shell: true}, AllowAllApproval).withRunner(
				func(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil },
			)
		}
	}
	if err := mgr.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	tools, err := mgr.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list err = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "shell.exec" {
		t.Fatalf("tools = %#v, want only shell.exec", tools)
	}
}

func TestConfig_ResolveServerNameOverride(t *testing.T) {
	cfg := Config{Browser: true, MCPServerNames: map[Kind]string{KindBrowser: "my-chrome"}}
	if got := cfg.resolveServerName(KindBrowser); got != "my-chrome" {
		t.Fatalf("override = %q, want my-chrome", got)
	}
	if got := cfg.resolveServerName(KindDesktop); got != "open-codex-computer-use" {
		t.Fatalf("default desktop = %q", got)
	}
}

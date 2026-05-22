package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, name string, m Manifest) {
	t.Helper()
	pdir := filepath.Join(dir, name)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManifestValidate(t *testing.T) {
	cases := []struct {
		name    string
		m       Manifest
		wantErr string
	}{
		{"missing name", Manifest{Version: "1"}, "name is required"},
		{"missing version", Manifest{Name: "p"}, "version is required"},
		{"bad hook", Manifest{Name: "p", Version: "1", Hooks: []HookBinding{{Event: "boom", Handler: "x"}}}, "unknown event"},
		{"missing handler", Manifest{Name: "p", Version: "1", Hooks: []HookBinding{{Event: HookBeforePrompt}}}, "handler is required"},
		{"dup virtual tool", Manifest{Name: "p", Version: "1", VirtualTools: []VirtualTool{{Name: "t", Response: "x"}, {Name: "t", Response: "y"}}}, "duplicate name"},
		{"alias missing field", Manifest{Name: "p", Version: "1", ToolAliases: []ToolAlias{{From: "a"}}}, "from and to"},
		{"ok", Manifest{Name: "p", Version: "1"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDiscoverAndShadowing(t *testing.T) {
	workspace := t.TempDir()
	user := t.TempDir()
	// Workspace has a "alpha" plugin; user also defines "alpha" (shadowed) and "beta".
	writeManifest(t, workspace, "alpha", Manifest{Name: "alpha", Version: "1.0.0"})
	writeManifest(t, user, "alpha", Manifest{Name: "alpha", Version: "0.9.0"})
	writeManifest(t, user, "beta", Manifest{Name: "beta", Version: "1.0.0"})

	manifests, err := Discover([]string{workspace, user})
	if err == nil || !strings.Contains(err.Error(), "shadowed") {
		t.Fatalf("expected shadow conflict, got %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("want 2 manifests, got %d", len(manifests))
	}
	if manifests[0].Name != "alpha" || manifests[0].Version != "1.0.0" {
		t.Fatalf("workspace alpha should win, got %+v", manifests[0])
	}
	if manifests[1].Name != "beta" {
		t.Fatalf("expected beta, got %+v", manifests[1])
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	manifests, err := Discover([]string{filepath.Join(t.TempDir(), "nope")})
	if err != nil {
		t.Fatalf("missing root should not error, got %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("want 0 manifests, got %d", len(manifests))
	}
}

func TestDispatchOrderInjectAndBlock(t *testing.T) {
	rt := NewRuntime("")
	rt.Load([]Manifest{
		{Name: "a", Version: "1", Hooks: []HookBinding{{Event: HookBeforePrompt, Handler: "inject"}}},
		{Name: "b", Version: "1", Hooks: []HookBinding{{Event: HookBeforePrompt, Handler: "block"}}},
		{Name: "c", Version: "1", Hooks: []HookBinding{{Event: HookBeforePrompt, Handler: "inject"}}},
	})
	rt.RegisterHandler("inject", func(_ context.Context, hc HookContext) (HookDecision, error) {
		return HookDecision{Inject: "from-" + hc.SessionID}, nil
	})
	rt.RegisterHandler("block", func(_ context.Context, hc HookContext) (HookDecision, error) {
		return HookDecision{Block: true, BlockReason: "no", Inject: "stop"}, nil
	})

	dec, err := rt.Dispatch(context.Background(), HookBeforePrompt, HookContext{SessionID: "s1"})
	if err != nil {
		t.Fatalf("dispatch err: %v", err)
	}
	if !dec.Block || dec.BlockReason != "no" {
		t.Fatalf("expected block decision, got %+v", dec)
	}
	if !strings.Contains(dec.Inject, "from-s1") || !strings.Contains(dec.Inject, "stop") {
		t.Fatalf("inject should accumulate before block, got %q", dec.Inject)
	}
}

func TestDispatchUnknownHandlerSurfacesError(t *testing.T) {
	rt := NewRuntime("")
	rt.Load([]Manifest{{Name: "x", Version: "1", Hooks: []HookBinding{{Event: HookAfterTurn, Handler: "ghost"}}}})
	_, err := rt.Dispatch(context.Background(), HookAfterTurn, HookContext{})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected unknown-handler error, got %v", err)
	}
}

func TestDispatchMatcher(t *testing.T) {
	rt := NewRuntime("")
	rt.Load([]Manifest{{Name: "p", Version: "1", Hooks: []HookBinding{
		{Event: HookBeforeDelegate, Handler: "h", Matcher: "bash"},
	}}})
	called := 0
	rt.RegisterHandler("h", func(_ context.Context, hc HookContext) (HookDecision, error) {
		called++
		return HookDecision{}, nil
	})
	_, _ = rt.Dispatch(context.Background(), HookBeforeDelegate, HookContext{ToolName: "read_file"})
	_, _ = rt.Dispatch(context.Background(), HookBeforeDelegate, HookContext{ToolName: "bash"})
	if called != 1 {
		t.Fatalf("matcher should fire once, got %d", called)
	}
}

func TestRenderVirtualTool(t *testing.T) {
	vt := VirtualTool{Name: "t", Response: "session={{.session_id}} who={{.input.who}}"}
	out := RenderVirtualTool(vt, "abc", map[string]any{"who": "alice"})
	if out != "session=abc who=alice" {
		t.Fatalf("unexpected render: %q", out)
	}
}

func TestStatePersistence(t *testing.T) {
	root := t.TempDir()
	rt := NewRuntime(root)
	rt.Load([]Manifest{{
		Name:    "p",
		Version: "1",
		Hooks:   []HookBinding{{Event: HookAfterTurn, Handler: "bump"}},
		State:   &StateBinding{Path: "state.json"},
	}})
	rt.RegisterHandler("bump", func(_ context.Context, hc HookContext) (HookDecision, error) {
		n, _ := hc.State["count"].(float64)
		return HookDecision{StateUpdates: map[string]any{"count": n + 1}}, nil
	})

	for i := 0; i < 3; i++ {
		if _, err := rt.Dispatch(context.Background(), HookAfterTurn, HookContext{SessionID: "s1"}); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "p", "s1.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out["count"].(float64) != 3 {
		t.Fatalf("want count=3, got %v", out["count"])
	}
}

func TestStateSessionIsolation(t *testing.T) {
	root := t.TempDir()
	rt := NewRuntime(root)
	rt.Load([]Manifest{{
		Name: "p", Version: "1",
		Hooks: []HookBinding{{Event: HookAfterTurn, Handler: "set"}},
		State: &StateBinding{Path: "s.json"},
	}})
	rt.RegisterHandler("set", func(_ context.Context, hc HookContext) (HookDecision, error) {
		return HookDecision{StateUpdates: map[string]any{"id": hc.SessionID}}, nil
	})
	_, _ = rt.Dispatch(context.Background(), HookAfterTurn, HookContext{SessionID: "alpha"})
	_, _ = rt.Dispatch(context.Background(), HookAfterTurn, HookContext{SessionID: "beta"})
	a, _ := os.ReadFile(filepath.Join(root, "p", "alpha.json"))
	b, _ := os.ReadFile(filepath.Join(root, "p", "beta.json"))
	if !strings.Contains(string(a), `"alpha"`) || !strings.Contains(string(b), `"beta"`) {
		t.Fatalf("state not isolated: a=%s b=%s", a, b)
	}
}

func TestStateRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	rt := NewRuntime(root)
	m := Manifest{Name: "p", State: &StateBinding{Path: "s.json"}}
	got := rt.stateFilePath(m, "../escape")
	if strings.Contains(got, "..") {
		t.Fatalf("traversal not sanitised: %s", got)
	}
}

func TestVirtualToolsAndAliasesAggregation(t *testing.T) {
	rt := NewRuntime("")
	rt.Load([]Manifest{
		{Name: "a", Version: "1",
			VirtualTools: []VirtualTool{{Name: "z", Response: "z"}, {Name: "a", Response: "a"}},
			ToolAliases:  []ToolAlias{{From: "bash", To: "shell"}},
			BlockedTools: []string{"rm"},
		},
		{Name: "b", Version: "1",
			BlockedTools: []string{"rm", "curl"},
		},
	})
	vts := rt.VirtualTools()
	if len(vts) != 2 || vts[0].Name != "a" || vts[1].Name != "z" {
		t.Fatalf("virtual tools not sorted: %+v", vts)
	}
	if blocked := rt.BlockedTools(); len(blocked) != 2 || blocked[0] != "curl" || blocked[1] != "rm" {
		t.Fatalf("blocked tools wrong: %+v", blocked)
	}
	if a := rt.ToolAliases(); len(a) != 1 || a[0].From != "bash" {
		t.Fatalf("aliases: %+v", a)
	}
}

func TestLoadManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "p", Manifest{
		Name: "p", Version: "1",
		Hooks:        []HookBinding{{Event: HookBeforePrompt, Handler: "h"}},
		VirtualTools: []VirtualTool{{Name: "echo", Response: "hi"}},
	})
	m, err := LoadManifest(filepath.Join(dir, "p", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Path != filepath.Join(dir, "p") {
		t.Fatalf("path not set: %s", m.Path)
	}
	if len(m.Hooks) != 1 || m.Hooks[0].Event != HookBeforePrompt {
		t.Fatalf("hooks lost: %+v", m.Hooks)
	}
}

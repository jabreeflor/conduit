package coding

import (
	"errors"
	"strings"
	"testing"
)

func TestDispatchNonSlashIsPassthrough(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{})
	res, isSlash, err := d.Dispatch("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isSlash {
		t.Fatalf("expected non-slash input, got slash result %+v", res)
	}
}

func TestDispatchHelpListsCanonicalCommands(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{})
	res, isSlash, err := d.Dispatch("/help")
	if err != nil || !isSlash {
		t.Fatalf("expected slash + nil err, got isSlash=%v err=%v", isSlash, err)
	}
	for _, must := range []string{"/help", "/status", "/compact", "/diff", "/agents", "/memory", "/context", "/mcp", "/clear"} {
		if !strings.Contains(res.Output, must) {
			t.Errorf("help missing %q\n%s", must, res.Output)
		}
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{})
	_, isSlash, err := d.Dispatch("/nope")
	if !isSlash {
		t.Fatalf("expected slash classification")
	}
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown error, got %v", err)
	}
}

func TestDispatchClearAndCompactSetSideEffects(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{})
	res, _, _ := d.Dispatch("/clear")
	if !res.ClearTranscript {
		t.Errorf("/clear should set ClearTranscript")
	}
	res, _, _ = d.Dispatch("/compact")
	if !res.CompactRequested {
		t.Errorf("/compact should set CompactRequested")
	}
}

func TestDispatchStatusRendersBudgetAndSession(t *testing.T) {
	sess := &Session{ID: "code-1-aa", RepositoryRoot: "/repo", Branch: "main"}
	b := NewBudget(1000)
	b.Observe(250, 50)
	d := NewSlashDispatcher(SlashContext{
		Session: sess, Budget: b, Model: "claude-3.5-sonnet", Account: "user@example", PermissionMode: "ask",
	})
	res, _, err := d.Dispatch("/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"code-1-aa", "/repo", "claude-3.5-sonnet", "user@example", "ask", "250/1000"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("missing %q in:\n%s", want, res.Output)
		}
	}
}

func TestDispatchContextRendersPercent(t *testing.T) {
	b := NewBudget(1000)
	b.Observe(800, 0)
	d := NewSlashDispatcher(SlashContext{Budget: b})
	res, _, err := d.Dispatch("/context")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "80.0%") {
		t.Errorf("expected percent in:\n%s", res.Output)
	}
	res2, _, _ := d.Dispatch("/token-budget")
	if res.Output != res2.Output {
		t.Errorf("/token-budget should alias /context")
	}
}

func TestDispatchListsItemsSorted(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{
		AgentNames:   []string{"reviewer", "implementer", "planner"},
		MCPNames:     []string{"computer-use"},
		HookNames:    nil,
		PlanItems:    []string{"step 1", "step 2"},
		TaskItems:    []string{"first task", "second task"},
		MemoryRules:  []string{"~/CLAUDE.md", "./.conduit/rules/style.md"},
		TrustedRoots: []string{"/repo"},
	})

	res, _, _ := d.Dispatch("/agents")
	if !strings.Contains(res.Output, "implementer") || !strings.Contains(res.Output, "planner") {
		t.Errorf("/agents missing entries:\n%s", res.Output)
	}
	// Sorted alphabetically.
	if strings.Index(res.Output, "implementer") > strings.Index(res.Output, "reviewer") {
		t.Errorf("/agents not sorted:\n%s", res.Output)
	}

	res, _, _ = d.Dispatch("/mcp")
	if !strings.Contains(res.Output, "computer-use") {
		t.Errorf("/mcp missing entry")
	}

	res, _, _ = d.Dispatch("/hooks")
	if !strings.Contains(res.Output, "no hooks") {
		t.Errorf("/hooks should report empty: %s", res.Output)
	}

	res, _, _ = d.Dispatch("/plan")
	if !strings.Contains(res.Output, "step 1") {
		t.Errorf("/plan missing step")
	}

	res, _, _ = d.Dispatch("/task-next")
	if !strings.Contains(res.Output, "first task") {
		t.Errorf("/task-next should pick first: %s", res.Output)
	}

	res, _, _ = d.Dispatch("/memory")
	if !strings.Contains(res.Output, "CLAUDE.md") {
		t.Errorf("/memory should list rules")
	}

	res, _, _ = d.Dispatch("/trust")
	if !strings.Contains(res.Output, "/repo") {
		t.Errorf("/trust should list roots")
	}
}

func TestDispatchAgentSwitchUnknown(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{AgentNames: []string{"reviewer"}})
	if _, _, err := d.Dispatch("/agent missing"); err == nil {
		t.Fatalf("expected error on unknown agent")
	}
	if _, _, err := d.Dispatch("/agent reviewer"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestDispatchSearchRequiresHandlerAndArgs(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{})
	if _, _, err := d.Dispatch("/search foo"); err == nil {
		t.Fatalf("expected error when handler unset")
	}
	called := ""
	d2 := NewSlashDispatcher(SlashContext{SearchHandler: func(q string) (string, error) {
		called = q
		return "results: " + q, nil
	}})
	if _, _, err := d2.Dispatch("/search"); err == nil {
		t.Fatalf("expected error on empty args")
	}
	res, _, err := d2.Dispatch("/search needle in haystack")
	if err != nil {
		t.Fatal(err)
	}
	if called != "needle in haystack" || !strings.Contains(res.Output, "results:") {
		t.Errorf("search not invoked correctly: called=%q out=%q", called, res.Output)
	}
}

func TestDispatchDiffAndConfigErrorsAndPipes(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{})
	if _, _, err := d.Dispatch("/diff"); err == nil {
		t.Errorf("expected /diff to error without provider")
	}
	dWithDiff := NewSlashDispatcher(SlashContext{DiffProvider: func() (string, error) { return "", nil }})
	res, _, err := dWithDiff.Dispatch("/diff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "no diff") {
		t.Errorf("expected (no diff): %s", res.Output)
	}

	dCfg := NewSlashDispatcher(SlashContext{ConfigDump: func() (string, error) { return "key=value", nil }})
	res, _, err = dCfg.Dispatch("/config")
	if err != nil || !strings.Contains(res.Output, "key=value") {
		t.Errorf("/config should print dump: %v %s", err, res.Output)
	}

	dErr := NewSlashDispatcher(SlashContext{DiffProvider: func() (string, error) { return "", errors.New("boom") }})
	if _, _, err := dErr.Dispatch("/diff"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected boom: %v", err)
	}
}

func TestDispatchForkAndResume(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{
		ForkHandler:    func() (string, error) { return "code-2-bb", nil },
		ResumeResolver: func(q string) (string, error) { return "code-3-cc", nil },
	})
	res, _, err := d.Dispatch("/fork")
	if err != nil || res.LoadSessionID != "code-2-bb" {
		t.Errorf("fork: %v %+v", err, res)
	}
	res, _, err = d.Dispatch("/resume foo")
	if err != nil || res.LoadSessionID != "code-3-cc" {
		t.Errorf("resume: %v %+v", err, res)
	}
}

func TestDispatchApprovalsAndRemoteAccount(t *testing.T) {
	d := NewSlashDispatcher(SlashContext{PermissionMode: "plan", Remote: "https://api.example", Account: "me"})
	res, _, _ := d.Dispatch("/approvals")
	if !strings.Contains(res.Output, "plan") {
		t.Errorf("/approvals missing tier: %s", res.Output)
	}
	res, _, _ = d.Dispatch("/permissions")
	if !strings.Contains(res.Output, "plan") {
		t.Errorf("/permissions should alias /approvals")
	}
	res, _, _ = d.Dispatch("/remote")
	if !strings.Contains(res.Output, "https://api.example") {
		t.Errorf("/remote: %s", res.Output)
	}
	res, _, _ = d.Dispatch("/account")
	if !strings.Contains(res.Output, "me") {
		t.Errorf("/account: %s", res.Output)
	}
}

func TestSlashCommandsCatalogStable(t *testing.T) {
	specs := SlashCommands()
	if len(specs) < 20 {
		t.Errorf("expected >=20 commands, got %d", len(specs))
	}
	// Ensure no duplicate names.
	seen := map[string]bool{}
	for _, s := range specs {
		if seen[s.Name] {
			t.Errorf("duplicate %s", s.Name)
		}
		seen[s.Name] = true
	}
}

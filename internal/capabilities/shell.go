package capabilities

import (
	"bytes"
	"context"
	"fmt"
	osexec "os/exec"
	"strings"
	"time"
)

// ShellCapability exposes a small set of terminal primitives backed by the
// host's local exec runtime. This is the "Shell" tier of PRD §6.8.
//
// The adapter intentionally exposes a single tool — shell.exec — rather than
// a sprawling surface. File ops and code execution all flow through one
// command, which keeps the per-app approval prompt simple ("allow shell to
// run this?") and matches how Codex/Claude expose their bash tool.
//
// The adapter uses Go's exec.CommandContext (which is execFile-style: no
// shell interpolation, args are passed positionally) so user-controlled
// arguments cannot become shell metacharacters.
type ShellCapability struct {
	approval Approval
	allow    map[string]struct{}
	app      string
	runner   commandRunner // injected for tests
	timeout  time.Duration
	ready    bool
}

// commandRunner abstracts os/exec.CommandContext so tests can stub command
// execution without touching the host.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// NewShellCapability constructs the Shell adapter.
func NewShellCapability(cfg Config, approval Approval) *ShellCapability {
	if approval == nil {
		approval = AllowAllApproval
	}
	app := cfg.ShellAppName
	if app == "" {
		app = "shell"
	}
	allow := make(map[string]struct{}, len(cfg.ShellAllowedCommands))
	for _, c := range cfg.ShellAllowedCommands {
		c = strings.TrimSpace(c)
		if c != "" {
			allow[c] = struct{}{}
		}
	}
	return &ShellCapability{
		approval: approval,
		allow:    allow,
		app:      app,
		runner:   defaultRunner,
		timeout:  30 * time.Second,
	}
}

// withRunner is a test hook that overrides the command runner.
func (s *ShellCapability) withRunner(fn commandRunner) *ShellCapability {
	s.runner = fn
	return s
}

// Kind reports KindShell.
func (s *ShellCapability) Kind() Kind { return KindShell }

// Init validates the adapter is ready. Shell has no remote dependency so this
// is effectively a flag flip.
func (s *ShellCapability) Init(_ context.Context) error {
	s.ready = true
	return nil
}

// ListTools returns the single shell.exec tool.
func (s *ShellCapability) ListTools(_ context.Context) ([]Tool, error) {
	if !s.ready {
		return nil, newUnavailable(KindShell, "adapter not initialized")
	}
	return []Tool{{
		Capability:  KindShell,
		Name:        "shell.exec",
		Description: "Run a shell command on the host. Output is captured and returned. Subject to per-app approval.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The executable to run (e.g. \"git\", \"ls\").",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional arguments passed to the command.",
				},
			},
			"required": []string{"command"},
		},
	}}, nil
}

// Dispatch handles shell.exec invocations.
func (s *ShellCapability) Dispatch(ctx context.Context, name string, args map[string]any) (Result, error) {
	if !s.ready {
		return Result{}, newUnavailable(KindShell, "adapter not initialized")
	}
	if name != "shell.exec" {
		return Result{}, fmt.Errorf("shell: unknown tool %q", name)
	}

	command, ok := args["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return Result{}, fmt.Errorf("shell: command is required and must be a string")
	}

	if len(s.allow) > 0 {
		if _, allowed := s.allow[command]; !allowed {
			return Result{}, fmt.Errorf("shell: command %q is not in the allowed list", command)
		}
	}

	rawArgs, _ := args["args"].([]any)
	cmdArgs := make([]string, 0, len(rawArgs))
	for _, a := range rawArgs {
		if s, ok := a.(string); ok {
			cmdArgs = append(cmdArgs, s)
		}
	}

	approved, err := s.approval.RequireApproval(ctx, s.app, command+" "+strings.Join(cmdArgs, " "))
	if err != nil {
		return Result{}, fmt.Errorf("shell: approval check failed: %w", err)
	}
	if !approved {
		return Result{Text: "shell command denied by user", IsError: true}, nil
	}

	runCtx := ctx
	if s.timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	out, err := s.runner(runCtx, command, cmdArgs...)
	if err != nil {
		return Result{Text: fmt.Sprintf("%s\n%s", strings.TrimSpace(string(out)), err.Error()), IsError: true}, nil
	}
	return Result{Text: strings.TrimRight(string(out), "\n")}, nil
}

// Shutdown is a no-op for Shell.
func (s *ShellCapability) Shutdown(_ context.Context) error {
	s.ready = false
	return nil
}

// defaultRunner runs a command via os/exec and returns combined stdout+stderr.
// It uses CommandContext (the execFile-style API: no shell interpolation,
// arguments are passed positionally) so caller-supplied input cannot become
// shell metacharacters.
func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.Bytes(), err
}

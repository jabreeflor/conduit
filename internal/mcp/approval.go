package mcp

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// ApprovalPolicy controls whether a tool invocation requires explicit user
// consent before it executes.
type ApprovalPolicy int

const (
	// ApprovalNever means the tool runs without asking — for read-only tools.
	ApprovalNever ApprovalPolicy = iota
	// ApprovalSideEffecting means the tool has side effects and requires a
	// one-time approval per session.
	ApprovalSideEffecting
	// ApprovalAlways means every invocation requires explicit approval.
	ApprovalAlways
)

// SideEffectingTools is the default set of tool name prefixes / exact names
// that Conduit considers side-effecting and requires approval for.
var SideEffectingTools = []string{
	"write_file",
	"delete_file",
	"execute",
	"run_command",
	"send_email",
	"post_",
	"create_",
	"update_",
	"delete_",
}

// ApprovalGate enforces per-session approval for side-effecting tools.
// It is safe for concurrent use.
type ApprovalGate struct {
	policy   func(toolName string) ApprovalPolicy
	approved map[string]bool // session-scoped approvals
	prompt   io.Writer       // where approval prompts are written
	confirm  func(ctx context.Context, tool string, args map[string]any) (bool, error)
}

// NewApprovalGate creates a gate that writes prompts to w and uses confirm to
// ask the user. If confirm is nil, all approvals default to denied (safe for
// tests).
func NewApprovalGate(w io.Writer, confirm func(ctx context.Context, tool string, args map[string]any) (bool, error)) *ApprovalGate {
	return &ApprovalGate{
		policy:   defaultPolicy,
		approved: make(map[string]bool),
		prompt:   w,
		confirm:  confirm,
	}
}

// Check returns whether the given tool call may proceed. For side-effecting
// tools it asks the user once per session and caches the decision.
func (g *ApprovalGate) Check(ctx context.Context, toolName string, args map[string]any) (bool, error) {
	switch g.policy(toolName) {
	case ApprovalNever:
		return true, nil
	case ApprovalAlways:
		return g.ask(ctx, toolName, args)
	case ApprovalSideEffecting:
		if g.approved[toolName] {
			return true, nil
		}
		ok, err := g.ask(ctx, toolName, args)
		if ok {
			g.approved[toolName] = true
		}
		return ok, err
	default:
		return false, fmt.Errorf("mcp: unknown approval policy for %q", toolName)
	}
}

func (g *ApprovalGate) ask(ctx context.Context, tool string, args map[string]any) (bool, error) {
	if g.confirm == nil {
		return false, nil
	}
	fmt.Fprintf(g.prompt, "MCP tool %q wants to run with args %v — allow? ", tool, args)
	return g.confirm(ctx, tool, args)
}

func defaultPolicy(toolName string) ApprovalPolicy {
	lower := strings.ToLower(toolName)
	for _, prefix := range SideEffectingTools {
		if strings.HasPrefix(lower, prefix) || lower == prefix {
			return ApprovalSideEffecting
		}
	}
	return ApprovalNever
}

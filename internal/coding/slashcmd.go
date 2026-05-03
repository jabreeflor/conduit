package coding

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SlashResult is the structured outcome of a slash command. Surfaces
// (TUI, CLI, GUI) decide how to render Output and act on the side-effect
// fields (LoadSessionID, ClearTranscript, etc.). Keeping this layer
// surface-agnostic lets the Codex-aligned command set be wired in by
// any caller without re-implementing parse logic.
type SlashResult struct {
	// Output is the human-readable text the surface should render.
	Output string
	// ClearTranscript signals the surface to clear its transcript view.
	// Set by /clear; the journal on disk is unaffected.
	ClearTranscript bool
	// CompactRequested signals the surface to run reactive compaction now.
	// Set by /compact regardless of current budget headroom.
	CompactRequested bool
	// LoadSessionID asks the surface to switch to a different session.
	// Set by /resume and /fork.
	LoadSessionID string
}

// SlashContext is the read-only view a slash command needs to render
// status. Surfaces inject whatever is available; nil fields are skipped
// gracefully so partial wiring is fine.
type SlashContext struct {
	Session *Session
	Budget  *Budget
	// MemoryRules is the discovered CLAUDE.md / .conduit/rules/*.md content
	// summary rendered by /memory. Surfaces with the rules-discovery wire
	// installed populate this; the dispatcher does not load files itself.
	MemoryRules []string
	// AgentNames is the list of registered agent profiles for /agents and
	// /agent.
	AgentNames []string
	// MCPNames is the list of registered MCP servers for /mcp.
	MCPNames []string
	// HookNames is the list of installed hook ids for /hooks.
	HookNames []string
	// PermissionMode is "ask", "plan", "yolo", etc. for /permissions and
	// /approvals.
	PermissionMode string
	// PlanItems and TaskItems back /plan and /tasks. Each item is one line.
	PlanItems []string
	TaskItems []string
	// Account is the displayable account / model identifier shown by
	// /account and /status.
	Account string
	// Model and Remote back /status and /remote.
	Model  string
	Remote string
	// PromptTemplates back /prompt list.
	PromptTemplates []string
	// TrustedRoots is the list of paths the user has trusted, shown by /trust.
	TrustedRoots []string
	// SearchHandler, when set, executes a free-form search query for /search.
	SearchHandler func(query string) (string, error)
	// ConfigDump returns a textual dump of the active config for /config.
	ConfigDump func() (string, error)
	// DiffProvider returns the working-tree diff for /diff.
	DiffProvider func() (string, error)
	// ReviewHandler runs a review pass for /review.
	ReviewHandler func(args []string) (string, error)
	// ForkHandler creates a fork from the current turn for /fork.
	ForkHandler func() (string, error)
	// ResumeResolver resolves a session id for /resume.
	ResumeResolver func(query string) (string, error)
}

// SlashDispatcher dispatches Codex-aligned slash commands. The dispatcher
// is intentionally small: it parses, validates, and renders the
// well-known set; anything that requires live state pulls from a
// SlashContext supplied by the caller.
type SlashDispatcher struct {
	Ctx SlashContext
}

// NewSlashDispatcher returns a dispatcher bound to ctx.
func NewSlashDispatcher(ctx SlashContext) *SlashDispatcher {
	return &SlashDispatcher{Ctx: ctx}
}

// Dispatch parses a single input line. Returns (result, true, err) when
// the line is a slash command, (zero, false, nil) when the line is not
// (caller should treat as a normal user turn).
func (d *SlashDispatcher) Dispatch(line string) (SlashResult, bool, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return SlashResult{}, false, nil
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return SlashResult{}, true, errors.New("empty slash command")
	}
	cmd := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	args := fields[1:]
	res, err := d.run(cmd, args)
	return res, true, err
}

// SlashCommands returns the canonical Codex-aligned command list with
// short descriptions. Used by /help and surface auto-complete.
func SlashCommands() []SlashCommandSpec {
	out := make([]SlashCommandSpec, len(slashCatalog))
	copy(out, slashCatalog)
	return out
}

// SlashCommandSpec is one entry in the catalog.
type SlashCommandSpec struct {
	Name string
	Help string
}

// slashCatalog enumerates every command from PRD §6.25.4 and §6.24.25
// in the order /help renders. Keep this list sorted by category, not
// alphabetically — the order is part of the user-visible help output.
var slashCatalog = []SlashCommandSpec{
	// Core conversation.
	{"/help", "show this command list"},
	{"/clear", "clear the transcript view (journal preserved)"},
	{"/status", "show model, account, and session status"},
	{"/resume", "resume a previous session by id or query"},
	{"/fork", "fork a new session from the current turn"},
	{"/compact", "force reactive context compaction now"},
	{"/diff", "show the working-tree diff"},
	{"/review", "run an automated code review pass"},

	// Permissions & approvals.
	{"/approvals", "show or change the active approvals tier"},
	{"/permissions", "alias for /approvals"},
	{"/trust", "list or modify trusted directory roots"},

	// Agent & memory plumbing.
	{"/agent", "switch the active agent profile"},
	{"/agents", "list available agent profiles"},
	{"/memory", "show discovered CLAUDE.md / .conduit/rules/*.md"},
	{"/hooks", "list installed hooks"},

	// Coding-engine extras.
	{"/context", "preflight token budget breakdown"},
	{"/token-budget", "alias for /context"},
	{"/mcp", "list registered MCP servers"},
	{"/search", "run a free-form search across the workspace"},
	{"/remote", "show the remote endpoint for the active session"},
	{"/account", "show the active account / credential id"},
	{"/config", "dump the active config"},
	{"/plan", "show or update the plan list"},
	{"/tasks", "show the task queue"},
	{"/task-next", "advance to the next task"},
	{"/prompt", "list saved prompt templates"},
}

func (d *SlashDispatcher) run(cmd string, args []string) (SlashResult, error) {
	switch cmd {
	case "help", "?":
		return SlashResult{Output: renderHelp()}, nil
	case "clear":
		return SlashResult{Output: "cleared", ClearTranscript: true}, nil
	case "status":
		return d.status()
	case "resume":
		return d.resume(args)
	case "fork":
		return d.fork()
	case "compact":
		return SlashResult{Output: "compaction requested", CompactRequested: true}, nil
	case "diff":
		return d.diff()
	case "review":
		return d.review(args)
	case "approvals", "permissions":
		return d.approvals()
	case "trust":
		return d.trust()
	case "agent":
		return d.agentSwitch(args)
	case "agents":
		return d.list("agents", d.Ctx.AgentNames)
	case "memory":
		return d.list("memory rules", d.Ctx.MemoryRules)
	case "hooks":
		return d.list("hooks", d.Ctx.HookNames)
	case "context", "token-budget":
		return d.context()
	case "mcp":
		return d.list("mcp servers", d.Ctx.MCPNames)
	case "search":
		return d.search(args)
	case "remote":
		return d.scalar("remote", d.Ctx.Remote)
	case "account":
		return d.scalar("account", d.Ctx.Account)
	case "config":
		return d.config()
	case "plan":
		return d.list("plan", d.Ctx.PlanItems)
	case "tasks":
		return d.list("tasks", d.Ctx.TaskItems)
	case "task-next":
		return d.taskNext()
	case "prompt":
		return d.list("prompts", d.Ctx.PromptTemplates)
	default:
		return SlashResult{}, fmt.Errorf("unknown slash command %q (try /help)", "/"+cmd)
	}
}

func renderHelp() string {
	specs := SlashCommands()
	width := 0
	for _, s := range specs {
		if len(s.Name) > width {
			width = len(s.Name)
		}
	}
	var b strings.Builder
	for _, s := range specs {
		fmt.Fprintf(&b, "%-*s  %s\n", width, s.Name, s.Help)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (d *SlashDispatcher) status() (SlashResult, error) {
	var b strings.Builder
	if d.Ctx.Session != nil {
		fmt.Fprintf(&b, "session: %s\n", d.Ctx.Session.ID)
		if d.Ctx.Session.RepositoryRoot != "" {
			fmt.Fprintf(&b, "repo:    %s (%s)\n", d.Ctx.Session.RepositoryRoot, d.Ctx.Session.Branch)
		}
		if d.Ctx.Session.WorktreeActive {
			fmt.Fprintf(&b, "wt:      %s [%s]\n", d.Ctx.Session.WorktreePath, d.Ctx.Session.WorktreeBranch)
		}
	}
	if d.Ctx.Model != "" {
		fmt.Fprintf(&b, "model:   %s\n", d.Ctx.Model)
	}
	if d.Ctx.Account != "" {
		fmt.Fprintf(&b, "account: %s\n", d.Ctx.Account)
	}
	if d.Ctx.PermissionMode != "" {
		fmt.Fprintf(&b, "tier:    %s\n", d.Ctx.PermissionMode)
	}
	if d.Ctx.Budget != nil {
		s := d.Ctx.Budget.Snapshot()
		fmt.Fprintf(&b, "budget:  %d/%d input tokens (compact=%t)\n", s.UsedInput, s.ModelInputWindow, s.ShouldCompact)
	}
	out := strings.TrimRight(b.String(), "\n")
	if out == "" {
		out = "no status available"
	}
	return SlashResult{Output: out}, nil
}

func (d *SlashDispatcher) context() (SlashResult, error) {
	if d.Ctx.Budget == nil {
		return SlashResult{Output: "no budget configured"}, nil
	}
	s := d.Ctx.Budget.Snapshot()
	pct := 0.0
	if s.ModelInputWindow > 0 {
		pct = float64(s.UsedInput) / float64(s.ModelInputWindow) * 100
	}
	out := fmt.Sprintf(
		"input window:  %d tokens\nused (input):  %d (%.1f%%)\nused (output): %d\nthreshold:     %.0f%%\nshould compact: %t",
		s.ModelInputWindow, s.UsedInput, pct, s.UsedOutput, s.Threshold*100, s.ShouldCompact,
	)
	return SlashResult{Output: out}, nil
}

func (d *SlashDispatcher) approvals() (SlashResult, error) {
	mode := d.Ctx.PermissionMode
	if mode == "" {
		mode = "(unset)"
	}
	return SlashResult{Output: "approvals tier: " + mode}, nil
}

func (d *SlashDispatcher) trust() (SlashResult, error) {
	return d.list("trusted roots", d.Ctx.TrustedRoots)
}

func (d *SlashDispatcher) agentSwitch(args []string) (SlashResult, error) {
	if len(args) == 0 {
		return d.list("agents", d.Ctx.AgentNames)
	}
	target := args[0]
	for _, name := range d.Ctx.AgentNames {
		if strings.EqualFold(name, target) {
			return SlashResult{Output: "agent: " + name}, nil
		}
	}
	return SlashResult{}, fmt.Errorf("unknown agent %q", target)
}

func (d *SlashDispatcher) list(label string, items []string) (SlashResult, error) {
	if len(items) == 0 {
		return SlashResult{Output: "no " + label}, nil
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d):\n", label, len(sorted))
	for _, it := range sorted {
		fmt.Fprintf(&b, "  - %s\n", it)
	}
	return SlashResult{Output: strings.TrimRight(b.String(), "\n")}, nil
}

func (d *SlashDispatcher) scalar(label, value string) (SlashResult, error) {
	if value == "" {
		return SlashResult{Output: "no " + label + " configured"}, nil
	}
	return SlashResult{Output: label + ": " + value}, nil
}

func (d *SlashDispatcher) search(args []string) (SlashResult, error) {
	if d.Ctx.SearchHandler == nil {
		return SlashResult{}, errors.New("search not wired (no SearchHandler)")
	}
	if len(args) == 0 {
		return SlashResult{}, errors.New("usage: /search <query>")
	}
	out, err := d.Ctx.SearchHandler(strings.Join(args, " "))
	if err != nil {
		return SlashResult{}, err
	}
	return SlashResult{Output: out}, nil
}

func (d *SlashDispatcher) config() (SlashResult, error) {
	if d.Ctx.ConfigDump == nil {
		return SlashResult{Output: "config dump not wired"}, nil
	}
	out, err := d.Ctx.ConfigDump()
	if err != nil {
		return SlashResult{}, err
	}
	return SlashResult{Output: out}, nil
}

func (d *SlashDispatcher) diff() (SlashResult, error) {
	if d.Ctx.DiffProvider == nil {
		return SlashResult{}, errors.New("diff not wired")
	}
	out, err := d.Ctx.DiffProvider()
	if err != nil {
		return SlashResult{}, err
	}
	if strings.TrimSpace(out) == "" {
		out = "(no diff)"
	}
	return SlashResult{Output: out}, nil
}

func (d *SlashDispatcher) review(args []string) (SlashResult, error) {
	if d.Ctx.ReviewHandler == nil {
		return SlashResult{}, errors.New("review not wired")
	}
	out, err := d.Ctx.ReviewHandler(args)
	if err != nil {
		return SlashResult{}, err
	}
	return SlashResult{Output: out}, nil
}

func (d *SlashDispatcher) fork() (SlashResult, error) {
	if d.Ctx.ForkHandler == nil {
		return SlashResult{}, errors.New("fork not wired")
	}
	id, err := d.Ctx.ForkHandler()
	if err != nil {
		return SlashResult{}, err
	}
	return SlashResult{Output: "forked -> " + id, LoadSessionID: id}, nil
}

func (d *SlashDispatcher) resume(args []string) (SlashResult, error) {
	if d.Ctx.ResumeResolver == nil {
		return SlashResult{}, errors.New("resume not wired")
	}
	query := ""
	if len(args) > 0 {
		query = strings.Join(args, " ")
	}
	id, err := d.Ctx.ResumeResolver(query)
	if err != nil {
		return SlashResult{}, err
	}
	return SlashResult{Output: "resumed " + id, LoadSessionID: id}, nil
}

func (d *SlashDispatcher) taskNext() (SlashResult, error) {
	if len(d.Ctx.TaskItems) == 0 {
		return SlashResult{Output: "no tasks queued"}, nil
	}
	return SlashResult{Output: "next: " + d.Ctx.TaskItems[0]}, nil
}

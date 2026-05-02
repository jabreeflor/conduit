package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jabreeflor/conduit/internal/coding"
	"github.com/jabreeflor/conduit/internal/computeruse"
	"github.com/jabreeflor/conduit/internal/config"
	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/endpoint"
	evalpkg "github.com/jabreeflor/conduit/internal/eval"
	"github.com/jabreeflor/conduit/internal/localmodel"
	"github.com/jabreeflor/conduit/internal/mcp"
	"github.com/jabreeflor/conduit/internal/router"
	"github.com/jabreeflor/conduit/internal/skills"
	"github.com/jabreeflor/conduit/internal/tools"
	"github.com/jabreeflor/conduit/internal/tui"
	"github.com/jabreeflor/conduit/internal/usage"
)

var version = "dev"

// registerBuiltinMCPServers wires Conduit subsystems that ship their own
// MCP servers (PRD §6.8 computer use today; more later) into the MCP
// runtime. User-supplied mcp.yaml entries always override these.
func registerBuiltinMCPServers() {
	cfg, err := config.Load()
	if err != nil {
		// Config errors are surfaced when callers actually need a
		// section — don't fail startup on a bad config here.
		return
	}
	cu := computeruse.FromRootConfig(cfg)
	if entry, ok := cu.ServerEntry(); ok {
		mcp.RegisterBuiltinServer(entry)
	}
}

func main() {
	registerBuiltinMCPServers()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "version":
			fmt.Printf("conduit %s\n", version)
			return
		case "mcp":
			if err := mcp.RunCLI(context.Background(), os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit mcp: %v\n", err)
				os.Exit(1)
			}
			return
		case "models":
			if err := localmodel.RunCLI(context.Background(), os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit models: %v\n", err)
				os.Exit(1)
			}
			return
		case "eval":
			if err := runEvalCLI(context.Background(), os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit eval: %v\n", err)
				os.Exit(1)
			}
			return
		case "usage":
			if err := usage.RunCLI(context.Background(), os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit usage: %v\n", err)
				os.Exit(1)
			}
			return
		case "computer-use":
			if err := computeruse.RunCLI(context.Background(), os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit computer-use: %v\n", err)
				os.Exit(1)
			}
			return
		case "skills":
			if err := runSkillsCLI(os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit skills: %v\n", err)
				os.Exit(1)
			}
			return
		case "code":
			if err := runCodeCLI(context.Background(), os.Args[2:], os.Stdin, os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit code: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := tui.RunInteractive(); err != nil {
		fmt.Fprintf(os.Stderr, "conduit: %v\n", err)
		os.Exit(1)
	}
}

func runEvalCLI(ctx context.Context, args []string, stdout, stderr *os.File) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: conduit eval replay <session-id> --model <model> [--diff]")
		return flag.ErrHelp
	}
	switch args[0] {
	case "run", "compare", "report":
		return evalpkg.RunCLI(ctx, args, stdout, stderr)
	case "replay":
		return runEvalReplay(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown eval command %q", args[0])
	}
}

func runEvalReplay(ctx context.Context, args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("conduit eval replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	model := fs.String("model", "", "target model")
	diff := fs.Bool("diff", false, "print output diffs")
	sessionsDir := fs.String("sessions-dir", "", "session JSONL directory")
	resultsDir := fs.String("results-dir", "", "eval results directory")
	flagArgs, positionalArgs, err := splitReplayArgs(args)
	if err != nil {
		return err
	}
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(positionalArgs) != 1 {
		return fmt.Errorf("usage: conduit eval replay <session-id> --model <model> [--diff]")
	}

	responder := evalpkg.Responder(evalpkg.SnapshotResponder{})
	if live, ok := providerResponderFromEnv(*model); ok {
		responder = live
	}

	summary, err := evalpkg.ReplaySession(ctx, evalpkg.ReplayOptions{
		SessionID:   positionalArgs[0],
		Model:       *model,
		Diff:        *diff,
		SessionsDir: *sessionsDir,
		ResultsDir:  *resultsDir,
		Responder:   responder,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Replay eval: %s -> %s\n", summary.SessionID, summary.Model)
	fmt.Fprintf(stdout, "Turns: %d, matches: %d, changed: %d\n", summary.Turns, summary.Matches, summary.Turns-summary.Matches)
	fmt.Fprintf(stdout, "Source: %s\n", summary.SourcePath)
	fmt.Fprintf(stdout, "Results: %s\n", summary.ResultPath)
	if summary.SnapshotMode {
		fmt.Fprintln(stdout, "Mode: snapshot (set OPENAI_API_KEY or ANTHROPIC_API_KEY for live model replay)")
	}
	for _, d := range summary.Diffs {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, d)
	}
	return nil
}

func splitReplayArgs(args []string) ([]string, []string, error) {
	stringFlags := map[string]bool{
		"--model":        true,
		"--sessions-dir": true,
		"--results-dir":  true,
	}
	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		name := arg
		if before, _, ok := strings.Cut(arg, "="); ok {
			name = before
		}
		if stringFlags[name] && !strings.Contains(arg, "=") {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	return flagArgs, positional, nil
}

type providerResponder struct {
	provider router.Provider
}

func (r providerResponder) Replay(ctx context.Context, req evalpkg.ReplayRequest) (evalpkg.ReplayResponse, error) {
	inputs := make([]router.Input, 0, len(req.History))
	for _, msg := range req.History {
		if msg.Content == "" {
			continue
		}
		inputs = append(inputs, router.Input{
			Type: router.InputText,
			Ref:  msg.Role,
			Text: fmt.Sprintf("%s: %s", msg.Role, msg.Content),
		})
	}
	resp, err := r.provider.Infer(ctx, router.Request{
		SessionID: req.SessionID,
		TaskType:  router.TaskGeneral,
		Inputs:    inputs,
		Prompt:    req.Prompt,
	})
	if err != nil {
		return evalpkg.ReplayResponse{}, err
	}
	return evalpkg.ReplayResponse{
		Output:       resp.Text,
		Provider:     resp.Provider,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}, nil
}

// runCodeCLI wires the `conduit code` REPL with tier-filtered tools, a
// budget tracker, and a fresh session journal. The provider streamer is
// stubbed to echo input until a real client lands; that swap is the only
// dependency between this entry point and the live coding agent.
func runCodeCLI(ctx context.Context, args []string, stdin, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("conduit code", flag.ContinueOnError)
	fs.SetOutput(stderr)
	allowWrite := fs.Bool("allow-write", false, "allow filesystem write tools (write_file, edit_file, notebook_edit)")
	allowShell := fs.Bool("allow-shell", false, "allow shell tools (bash)")
	maxInputTokens := fs.Int("max-input-tokens", 200_000, "model input window for context budgeting")
	if err := fs.Parse(args); err != nil {
		return err
	}

	perms := contracts.CodingPermissions{
		AllowWrite: *allowWrite,
		AllowShell: *allowShell,
	}
	codingTools := coding.RegisterCodingTools(coding.DefaultCodingTools(), perms)
	// Pipeline is built so the REPL can resolve calls when real runners
	// arrive; PolicyConfig{} defers policy work to PR #62.
	_ = tools.NewPipeline(codingTools, tools.PolicyConfig{})

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}
	session, err := coding.NewSession(home)
	if err != nil {
		return err
	}
	budget := coding.NewBudget(*maxInputTokens)

	repl := &coding.REPL{
		Session:   session,
		Budget:    budget,
		Tools:     codingTools,
		Streamer:  echoStreamer{},
		Continuer: coding.DefaultContinuer{},
		In:        stdin,
		Out:       stdout,
	}
	fmt.Fprintf(stdout, "conduit code: session %s (allow-write=%t allow-shell=%t)\n", session.ID, perms.AllowWrite, perms.AllowShell)
	return repl.Run(ctx)
}

// echoStreamer is the placeholder Streamer: it echoes the user's prompt
// back as the assistant turn with a natural finishReason so the REPL skips
// auto-continuation. Replaced by a real provider client in the follow-up
// streaming PR.
type echoStreamer struct{}

func (echoStreamer) Stream(_ context.Context, prompt string, onDelta func(string)) (string, string, error) {
	out := "echo: " + prompt
	if onDelta != nil {
		onDelta(out)
	}
	return out, "stop", nil
}

func providerResponderFromEnv(model string) (evalpkg.Responder, bool) {
	switch {
	case strings.HasPrefix(model, "gpt-"):
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, false
		}
		return providerResponder{provider: endpoint.New(endpoint.Config{
			Name:    "openai",
			Type:    endpoint.TypeOpenAICompat,
			BaseURL: "https://api.openai.com",
			APIKey:  key,
			Model:   model,
		})}, true
	case strings.HasPrefix(model, "claude-"):
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, false
		}
		return providerResponder{provider: endpoint.New(endpoint.Config{
			Name:   "anthropic",
			Type:   endpoint.TypeAnthropic,
			APIKey: key,
			Model:  model,
		})}, true
	default:
		return nil, false
	}
}

// runSkillsCLI loads the skill registry on demand and dispatches the simple
// list/lookup/search verbs. The CLI is intentionally thin — full task-start
// integration lives in the agent loop, not here.
func runSkillsCLI(args []string, stdout, stderr *os.File) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: conduit skills <list|lookup|search> [args...]")
		return flag.ErrHelp
	}

	registry, err := newSkillsRegistry()
	if err != nil {
		return err
	}

	switch args[0] {
	case "list":
		printSkillRows(stdout, registry.List())
		return nil
	case "lookup":
		if len(args) < 2 {
			return fmt.Errorf("usage: conduit skills lookup <name>")
		}
		skill, ok := registry.Lookup(args[1])
		if !ok {
			fmt.Fprintln(stderr, "not found")
			os.Exit(1)
		}
		fmt.Fprintln(stdout, skill.Body)
		return nil
	case "search":
		if len(args) < 2 {
			return fmt.Errorf("usage: conduit skills search <query>")
		}
		printSkillRows(stdout, registry.Search(strings.Join(args[1:], " ")))
		return nil
	default:
		return fmt.Errorf("unknown skills command %q", args[0])
	}
}

func newSkillsRegistry() (*skills.Registry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// A missing home dir is unusual but should not break the CLI; the
		// registry will simply have no personal/imported/bundled tiers.
		home = ""
	}
	workspace, err := os.Getwd()
	if err != nil {
		workspace = ""
	}
	registry := skills.NewRegistry(skills.DefaultRoots(home, workspace))
	if err := registry.Load([]skills.Adapter{skills.NewMarkdownAdapter()}); err != nil {
		return nil, err
	}
	return registry, nil
}

func printSkillRows(out *os.File, list []contracts.Skill) {
	for _, skill := range list {
		fmt.Fprintf(out, "%s\t%s\t%s\n", skill.Tier, skill.Name, truncate(skill.Description, 80))
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

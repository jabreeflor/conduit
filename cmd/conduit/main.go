package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jabreeflor/conduit/internal/endpoint"
	evalpkg "github.com/jabreeflor/conduit/internal/eval"
	"github.com/jabreeflor/conduit/internal/mcp"
	"github.com/jabreeflor/conduit/internal/router"
	"github.com/jabreeflor/conduit/internal/tui"
)

var version = "dev"

func main() {
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
		case "eval":
			if err := runEvalCLI(context.Background(), os.Args[2:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintf(os.Stderr, "conduit eval: %v\n", err)
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

package coding

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ExecResult is the structured outcome of one `conduit exec` run.
// Designed for CI/CD consumers that need a stable, scriptable contract:
// every field is JSON-tagged and the schema is versioned.
type ExecResult struct {
	SchemaVersion string    `json:"schema_version"`
	SessionID     string    `json:"session_id"`
	Prompt        string    `json:"prompt"`
	Output        string    `json:"output"`
	FinishReason  string    `json:"finish_reason"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	DurationMS    int64     `json:"duration_ms"`
	Turns         int       `json:"turns"`
	BudgetUsed    int       `json:"budget_input_tokens_used"`
	BudgetWindow  int       `json:"budget_input_window"`
	ExitCode      int       `json:"exit_code"`
}

// ExecOptions controls one execution. Either Prompt or PromptFile must be
// set; PromptFile takes precedence if both are provided. When neither is
// set the runner reads stdin.
type ExecOptions struct {
	Prompt          string
	PromptFile      string
	Format          string // "text" | "json"
	MaxInputTokens  int
	AllowWrite      bool
	AllowShell      bool
	HomeDir         string
	WorkingDir      string
	Streamer        Streamer
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	NoSession       bool // skip writing the session journal (for ephemeral CI runs)
	MaxAutoContinue int
}

// RunExec executes a single non-interactive coding turn end-to-end and
// returns a structured result. The intended callers are CI/CD scripts and
// pre-commit hooks where deterministic output and a clean exit code are
// required. Interactive use stays in the REPL.
func RunExec(ctx context.Context, opts ExecOptions) (ExecResult, error) {
	prompt, err := resolvePrompt(opts)
	if err != nil {
		return ExecResult{}, err
	}
	if opts.Streamer == nil {
		return ExecResult{}, errors.New("exec: streamer required")
	}
	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ExecResult{}, fmt.Errorf("exec: resolve home: %w", err)
		}
		opts.HomeDir = home
	}
	if opts.MaxInputTokens <= 0 {
		opts.MaxInputTokens = 200_000
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}

	res := ExecResult{
		SchemaVersion: "conduit-exec/1",
		Prompt:        prompt,
		StartedAt:     time.Now().UTC(),
		BudgetWindow:  opts.MaxInputTokens,
	}

	var sessID string
	var session *Session
	if !opts.NoSession {
		s, err := NewSessionInDir(opts.HomeDir, opts.WorkingDir)
		if err != nil {
			return res, err
		}
		session = s
		sessID = s.ID
		if _, err := s.Append(contracts.CodingTurn{Role: "user", Content: prompt}); err != nil {
			return res, err
		}
	}
	res.SessionID = sessID

	budget := NewBudget(opts.MaxInputTokens)

	full, finish, err := opts.Streamer.Stream(ctx, prompt, nil)
	if err != nil {
		res.FinishedAt = time.Now().UTC()
		res.DurationMS = res.FinishedAt.Sub(res.StartedAt).Milliseconds()
		res.ExitCode = 1
		return res, err
	}
	if session != nil {
		_, _ = session.Append(contracts.CodingTurn{Role: "assistant", Content: full})
	}
	budget.Observe(estimateTokens(prompt), estimateTokens(full))

	res.Output = full
	res.FinishReason = finish
	res.Turns = 1
	res.BudgetUsed = budget.Snapshot().UsedInput
	res.FinishedAt = time.Now().UTC()
	res.DurationMS = res.FinishedAt.Sub(res.StartedAt).Milliseconds()
	res.ExitCode = 0
	return res, nil
}

func resolvePrompt(opts ExecOptions) (string, error) {
	if opts.PromptFile != "" {
		b, err := os.ReadFile(opts.PromptFile)
		if err != nil {
			return "", fmt.Errorf("exec: read prompt file: %w", err)
		}
		return strings.TrimRight(string(b), "\n"), nil
	}
	if opts.Prompt != "" {
		return opts.Prompt, nil
	}
	if opts.Stdin == nil {
		return "", errors.New("exec: no prompt provided (use --prompt, --prompt-file, or pipe stdin)")
	}
	b, err := io.ReadAll(opts.Stdin)
	if err != nil {
		return "", fmt.Errorf("exec: read stdin: %w", err)
	}
	out := strings.TrimRight(string(b), "\n")
	if out == "" {
		return "", errors.New("exec: empty prompt on stdin")
	}
	return out, nil
}

// RenderExecResult writes the result in the requested format. "json"
// emits a single line of JSON (one record per invocation, friendly to
// CI log-aggregators); anything else falls back to a plain-text summary
// followed by the model output on its own line.
func RenderExecResult(w io.Writer, res ExecResult, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	default:
		fmt.Fprintf(w, "session: %s\n", res.SessionID)
		fmt.Fprintf(w, "finish:  %s (turns=%d, tokens_in=%d/%d, %dms)\n",
			res.FinishReason, res.Turns, res.BudgetUsed, res.BudgetWindow, res.DurationMS)
		fmt.Fprintln(w, "---")
		fmt.Fprintln(w, res.Output)
		return nil
	}
}

// ParseExecArgs parses the `conduit exec` flag set. Exposed for tests
// and for callers that want to validate args without executing.
func ParseExecArgs(args []string) (ExecOptions, error) {
	fs := flag.NewFlagSet("conduit exec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prompt := fs.String("prompt", "", "prompt text (mutually exclusive with --prompt-file)")
	promptFile := fs.String("prompt-file", "", "read prompt from file ('-' for stdin)")
	format := fs.String("format", "text", "output format: text | json")
	maxInput := fs.Int("max-input-tokens", 200_000, "model input window for context budgeting")
	allowWrite := fs.Bool("allow-write", false, "allow filesystem write tools")
	allowShell := fs.Bool("allow-shell", false, "allow shell tools")
	noSession := fs.Bool("no-session", false, "skip writing to session journal")
	cwd := fs.String("cwd", "", "working directory (defaults to process cwd)")
	if err := fs.Parse(args); err != nil {
		return ExecOptions{}, err
	}
	if *prompt != "" && *promptFile != "" {
		return ExecOptions{}, errors.New("--prompt and --prompt-file are mutually exclusive")
	}
	switch strings.ToLower(*format) {
	case "", "text", "json":
	default:
		return ExecOptions{}, fmt.Errorf("unknown --format %q (use text or json)", *format)
	}
	return ExecOptions{
		Prompt:         *prompt,
		PromptFile:     *promptFile,
		Format:         *format,
		MaxInputTokens: *maxInput,
		AllowWrite:     *allowWrite,
		AllowShell:     *allowShell,
		NoSession:      *noSession,
		WorkingDir:     *cwd,
	}, nil
}

package computeruse

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jabreeflor/conduit/internal/computeruse/permissions"
)

// RunCLI is the entry point for `conduit computer-use`. It manages the
// persistent per-app approval store: list, approve, revoke, and check.
func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		return printUsage(stdout)
	}

	switch args[0] {
	case "list":
		return runList(args[1:], stdout, stderr)
	case "approve":
		return runApprove(ctx, args[1:], stdout, stderr)
	case "revoke":
		return runRevoke(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "permissions":
		return permissions.RunCLI(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown computer-use subcommand %q; try: list, approve, revoke, check, permissions", args[0])
	}
}

func printUsage(w io.Writer) error {
	_, err := fmt.Fprint(w, `usage: conduit computer-use <command>

Commands:
  list                       show every approval (active + revoked) and its scope
  approve --bundle <id>      grant computer-use permission for an app
          [--name <name>]
          [--scope full|read_only]
          [--note <text>]
          [--yes]            skip the interactive y/N prompt
  revoke  --bundle <id>      revoke a previously granted approval
          [--name <name>]
  check   --bundle <id>      exit 0 if the app is approved, 1 otherwise
          [--name <name>]
  permissions [check|open <permission>|verify]
                             macOS Screen Recording + Accessibility flow

Approvals persist to ~/.conduit/approved-apps.json and survive across runs.
`)
	return err
}

func openStore(stderr io.Writer) (*Store, error) {
	store, err := OpenDefault()
	if err != nil {
		fmt.Fprintf(stderr, "computer-use: open store: %v\n", err)
		return nil, err
	}
	return store, nil
}

func runList(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("computer-use list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	activeOnly := fs.Bool("active", false, "show only currently-granting approvals")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := openStore(stderr)
	if err != nil {
		return err
	}

	var records []ApprovalRecord
	if *activeOnly {
		records = store.ActiveApprovals()
	} else {
		records = store.List()
	}
	if len(records) == 0 {
		fmt.Fprintln(stdout, "no approvals on file.")
		return nil
	}

	fmt.Fprintf(stdout, "%s\n", store.Path())
	for _, rec := range records {
		state := "active "
		when := rec.ApprovedAt.Format("2006-01-02")
		if !rec.Active() {
			state = "revoked"
			if rec.RevokedAt != nil {
				when = rec.RevokedAt.Format("2006-01-02")
			}
		}
		note := ""
		if rec.Note != "" {
			note = "  -- " + rec.Note
		}
		fmt.Fprintf(stdout, "  [%s] %s  scope=%s  %s%s\n",
			state, displayName(rec.App), rec.Scope, when, note)
	}
	return nil
}

func runApprove(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("computer-use approve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundle := fs.String("bundle", "", "macOS bundle ID (preferred)")
	name := fs.String("name", "", "fallback app display name")
	scope := fs.String("scope", string(ScopeFull), "approval scope: full | read_only")
	note := fs.String("note", "", "optional note recorded in the audit trail")
	yes := fs.Bool("yes", false, "skip the interactive confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}

	app := AppRef{BundleID: *bundle, Name: *name}
	if app.IsZero() {
		return fmt.Errorf("computer-use approve: --bundle or --name is required")
	}
	parsed, err := parseScope(*scope)
	if err != nil {
		return err
	}

	store, err := openStore(stderr)
	if err != nil {
		return err
	}

	if !*yes {
		ok, err := confirmInteractive(ctx, stdout, app, parsed)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stdout, "approval declined.")
			return nil
		}
	}

	rec, err := store.Approve(app, parsed, *note)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "approved %s with scope=%s (saved to %s).\n",
		displayName(rec.App), rec.Scope, store.Path())
	return nil
}

func runRevoke(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("computer-use revoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundle := fs.String("bundle", "", "macOS bundle ID")
	name := fs.String("name", "", "fallback app display name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	app := AppRef{BundleID: *bundle, Name: *name}
	if app.IsZero() {
		return fmt.Errorf("computer-use revoke: --bundle or --name is required")
	}

	store, err := openStore(stderr)
	if err != nil {
		return err
	}
	rec, err := store.Revoke(app)
	if err != nil {
		if errors.Is(err, ErrUnknownApp) {
			fmt.Fprintf(stderr, "no approval on file for %s.\n", displayName(app))
		}
		return err
	}
	fmt.Fprintf(stdout, "revoked %s (was scope=%s).\n", displayName(rec.App), rec.Scope)
	return nil
}

func runCheck(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("computer-use check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundle := fs.String("bundle", "", "macOS bundle ID")
	name := fs.String("name", "", "fallback app display name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	app := AppRef{BundleID: *bundle, Name: *name}
	if app.IsZero() {
		return fmt.Errorf("computer-use check: --bundle or --name is required")
	}

	store, err := openStore(stderr)
	if err != nil {
		return err
	}
	if store.IsApproved(app) {
		fmt.Fprintf(stdout, "approved: %s\n", displayName(app))
		return nil
	}
	fmt.Fprintf(stdout, "not approved: %s\n", displayName(app))
	// Mirrors `git diff --exit-code`: non-zero exit is the actionable signal
	// for shell scripts. The CLI caller maps this error to os.Exit(1).
	return ErrNotApproved
}

func parseScope(raw string) (Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "full":
		return ScopeFull, nil
	case "read_only", "readonly", "ro":
		return ScopeReadOnly, nil
	default:
		return "", fmt.Errorf("computer-use: unknown scope %q (want full | read_only)", raw)
	}
}

func confirmInteractive(ctx context.Context, w io.Writer, app AppRef, scope Scope) (bool, error) {
	if !isTerminal(os.Stdin) {
		return false, fmt.Errorf("computer-use approve: stdin is not a terminal; pass --yes to confirm non-interactively")
	}
	fmt.Fprintf(w, "Grant Conduit %s access to %s? [y/N] ", scope, displayName(app))

	type result struct {
		answer string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		ch <- result{answer: strings.TrimSpace(line), err: err}
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case r := <-ch:
		if r.err != nil && !errors.Is(r.err, io.EOF) {
			return false, r.err
		}
		ans := strings.ToLower(r.answer)
		return ans == "y" || ans == "yes", nil
	}
}

// isTerminal reports whether f looks like an interactive TTY. We avoid pulling
// in an extra dep (golang.org/x/term) by sniffing the file mode — good enough
// for the CLI prompt path. Tests bypass this branch via --yes.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

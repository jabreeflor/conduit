// cli.go — `conduit sandbox` subcommand surface.
//
// Verbs: list / create / switch / clone / destroy (PRD §15.7) plus the
// already-supported snapshot / rollback verbs from §15.6.
//
// The CLI layer is deliberately thin: each verb maps to one Manager call,
// and human-readable output goes to stdout while diagnostics go to stderr.
package sandbox

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// RunCLI is the entry point wired from cmd/conduit/main.go. It dispatches
// args[0] to the matching subcommand against a default-rooted Manager.
func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return dispatchCLI(ctx, NewManager(""), args, stdout, stderr)
}

// dispatchCLI is the inner dispatcher that drives RunCLI; it lets tests
// supply a tempdir-rooted Manager without touching $HOME.
func dispatchCLI(_ context.Context, mgr *Manager, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return flag.ErrHelp
	}

	switch args[0] {
	case "list":
		return runList(mgr, args[1:], stdout, stderr)
	case "create":
		return runCreate(mgr, args[1:], stdout, stderr)
	case "switch":
		return runSwitch(mgr, args[1:], stdout, stderr)
	case "clone":
		return runClone(mgr, args[1:], stdout, stderr)
	case "destroy":
		return runDestroy(mgr, args[1:], stdout, stderr)
	case "active":
		return runActive(mgr, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		fmt.Fprintf(stderr, "unknown sandbox command %q\n\n", args[0])
		printUsage(stderr)
		return fmt.Errorf("unknown sandbox command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: conduit sandbox <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  list                            list all sandboxes (active row marked with '*')")
	fmt.Fprintln(w, "  create  <name> [flags]          create a new sandbox")
	fmt.Fprintln(w, "  switch  <name>                  set the active sandbox")
	fmt.Fprintln(w, "  clone   <src> <dst> [flags]     duplicate an existing sandbox")
	fmt.Fprintln(w, "  destroy <name>                  delete a sandbox and all its data")
	fmt.Fprintln(w, "  active                          print the active sandbox name")
}

func runList(mgr *Manager, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit sandbox list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	infos, err := mgr.List()
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(stdout, "no sandboxes found")
		fmt.Fprintf(stdout, "  root: %s\n", mgr.Root())
		return nil
	}
	fmt.Fprintf(stdout, "%-2s  %-24s  %-19s  %-10s  %-10s  %s\n",
		"", "NAME", "LAST USED", "QUOTA", "MEMORY", "CPU")
	for _, info := range infos {
		marker := "  "
		if info.Active {
			marker = "* "
		}
		fmt.Fprintf(stdout, "%s  %-24s  %-19s  %-10s  %-10s  %s\n",
			marker,
			info.Name,
			formatTime(info.LastUsedAt),
			humanBytes(info.QuotaBytes),
			humanBytes(info.MemoryBytes),
			formatCPU(info.CPULimit),
		)
	}
	return nil
}

func runCreate(mgr *Manager, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit sandbox create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	quota := fs.Int64("quota-bytes", 0, "disk quota in bytes (default 10 GiB)")
	memory := fs.Int64("memory-bytes", 0, "process memory limit in bytes (default 4 GiB)")
	cpu := fs.Float64("cpu-limit", -1, "CPU allowance in fractional cores (0 = unconstrained)")
	makeActive := fs.Bool("activate", false, "set the new sandbox as active after creation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: conduit sandbox create <name> [flags]")
	}
	name := fs.Arg(0)

	opts := CreateOptions{
		QuotaBytes:  *quota,
		MemoryBytes: *memory,
	}
	if *cpu >= 0 {
		opts.CPULimit = *cpu
	}

	ws, err := mgr.Create(name, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created sandbox %q at %s\n", ws.Name(), ws.Root())

	if *makeActive {
		if err := mgr.SetActive(ws.Name()); err != nil {
			return fmt.Errorf("set active: %w", err)
		}
		fmt.Fprintf(stdout, "active sandbox: %s\n", ws.Name())
	}
	return nil
}

func runSwitch(mgr *Manager, args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: conduit sandbox switch <name>")
	}
	ws, err := mgr.Switch(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "switched to sandbox %q\n", ws.Name())
	return nil
}

func runClone(mgr *Manager, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit sandbox clone", flag.ContinueOnError)
	fs.SetOutput(stderr)
	quota := fs.Int64("quota-bytes", 0, "override disk quota for the clone (default: inherit)")
	memory := fs.Int64("memory-bytes", 0, "override memory limit for the clone (default: inherit)")
	cpu := fs.Float64("cpu-limit", 0, "override CPU allowance for the clone (default: inherit)")
	makeActive := fs.Bool("activate", false, "set the clone as active after cloning")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: conduit sandbox clone <src> <dst> [flags]")
	}

	opts := CloneOptions{
		QuotaBytes:  *quota,
		MemoryBytes: *memory,
		CPULimit:    *cpu,
	}
	ws, err := mgr.Clone(fs.Arg(0), fs.Arg(1), opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "cloned %q -> %q (%s)\n", fs.Arg(0), ws.Name(), ws.Root())

	if *makeActive {
		if err := mgr.SetActive(ws.Name()); err != nil {
			return fmt.Errorf("set active: %w", err)
		}
		fmt.Fprintf(stdout, "active sandbox: %s\n", ws.Name())
	}
	return nil
}

func runDestroy(mgr *Manager, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit sandbox destroy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "skip the safety prompt (no-op when stdin is non-interactive)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: conduit sandbox destroy <name> [--force]")
	}
	name := fs.Arg(0)

	if !*force && isInteractive() {
		fmt.Fprintf(stderr, "destroy sandbox %q? [y/N]: ", name)
		var resp string
		fmt.Fscanln(os.Stdin, &resp)
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "y" && resp != "yes" {
			fmt.Fprintln(stdout, "aborted")
			return nil
		}
	}

	if err := mgr.Destroy(name); err != nil {
		return err
	}
	// Clear the active pointer if we just destroyed it so the user is not
	// left with a dangling reference.
	if active, err := mgr.Active(); err == nil && active == name {
		_ = mgr.ClearActive()
	}
	fmt.Fprintf(stdout, "destroyed sandbox %q\n", name)
	return nil
}

func runActive(mgr *Manager, stdout, stderr io.Writer) error {
	name, err := mgr.Active()
	if err != nil {
		if errors.Is(err, ErrNoActive) {
			fmt.Fprintln(stdout, "no active sandbox")
			return nil
		}
		// ErrNotFound here means the active pointer is dangling.
		if errors.Is(err, ErrNotFound) {
			fmt.Fprintf(stderr, "active sandbox %q no longer exists; use `conduit sandbox switch` to pick another\n", name)
			return err
		}
		return err
	}
	fmt.Fprintln(stdout, name)
	return nil
}

// formatTime renders t in a compact human form. The zero value renders as
// "—" so listings stay aligned.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// humanBytes renders n as a short binary-prefixed string. 0 renders as "—"
// so unset limits read clearly in tabular output.
func humanBytes(n int64) string {
	if n <= 0 {
		return "—"
	}
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := unit, 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// formatCPU renders cpu as fractional cores. 0 renders as "—" to mean
// "unconstrained" so the user can see at a glance which sandboxes are capped.
func formatCPU(cpu float64) string {
	if cpu <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.2f", cpu)
}

// isInteractive returns true when stdin is attached to a terminal. We use
// stat() instead of x/term so the package keeps zero deps.
func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

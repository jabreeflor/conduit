package permissions

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// RunCLI is the entry point for `conduit computer-use permissions ...`.
// It supports three subcommands:
//
//	check       — print the current state of every required permission
//	open <perm> — launch the System Settings deep-link for one permission
//	verify      — check, then poll until every grant is present (or timeout)
//
// The CLI is mostly for doctor-style debugging; the surfaces (TUI/GUI) drive
// the same Manager directly so they can render rich UI.
func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runCheck(ctx, stdout)
	}
	switch args[0] {
	case "check", "status":
		return runCheck(ctx, stdout)
	case "open":
		return runOpen(ctx, args[1:], stdout, stderr)
	case "verify":
		return runVerify(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "usage: conduit computer-use permissions [check|open <permission>|verify]")
		return flag.ErrHelp
	}
}

func runCheck(ctx context.Context, stdout io.Writer) error {
	mgr := NewManager()
	report := mgr.Report(ctx)
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PERMISSION\tSTATE\tSETTINGS URL")
	for _, s := range report.Statuses {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Permission, s.State, s.SettingsURL)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if !report.AllGranted {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Some permissions are missing. Run:")
		fmt.Fprintln(stdout, "  conduit computer-use permissions open <permission>")
		fmt.Fprintln(stdout, "to launch the System Settings pane that grants it.")
	}
	return nil
}

func runOpen(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: conduit computer-use permissions open <screen_recording|accessibility>")
		return flag.ErrHelp
	}
	perm := contracts.ComputerUsePermission(strings.ToLower(args[0]))
	mgr := NewManager()
	if err := mgr.OpenSettings(ctx, perm); err != nil {
		if errors.Is(err, ErrUnsupportedPermission) {
			return fmt.Errorf("unknown permission %q (try: screen_recording, accessibility)", args[0])
		}
		return fmt.Errorf("open settings: %w", err)
	}
	fmt.Fprintf(stdout, "Opened System Settings for %s.\n", perm)
	fmt.Fprintln(stdout, "After granting, run: conduit computer-use permissions verify")
	return nil
}

func runVerify(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	mgr := NewManager()
	report := mgr.Report(ctx)
	if report.AllGranted {
		fmt.Fprintln(stdout, "All required permissions are granted.")
		return nil
	}
	for _, s := range report.Statuses {
		if s.State == contracts.ComputerUsePermissionStateGranted {
			continue
		}
		if s.State == contracts.ComputerUsePermissionStateNotApplicable {
			continue
		}
		fmt.Fprintf(stdout, "Waiting for grant: %s … (deadline %s)\n", s.Permission, mgr.verifyT)
		ok, last := mgr.VerifyAfterGrant(ctx, s.Permission)
		if !ok {
			return fmt.Errorf("permission %s still %s after %s — grant in System Settings and retry", s.Permission, last.State, mgr.verifyT)
		}
		fmt.Fprintf(stdout, "%s: granted\n", s.Permission)
	}
	fmt.Fprintln(stdout, "All required permissions are granted.")
	return nil
}

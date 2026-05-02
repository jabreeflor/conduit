package usage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RunCLI is the entry point for `conduit usage`.
func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return printUsage(stdout)
	}
	dir, err := defaultLogDirAbs()
	if err != nil {
		return err
	}

	switch args[0] {
	case "purge":
		return runPurge(args[1:], stdout, stderr, dir)
	case "export":
		return runExport(args[1:], stdout, stderr, dir)
	case "report":
		return runReport(args[1:], stdout, stderr, dir)
	case "dashboard":
		return runDashboard(args[1:], stdout, stderr, dir)
	case "-h", "--help", "help":
		return printUsage(stdout)
	default:
		return fmt.Errorf("unknown usage subcommand %q; try: purge, export, report, dashboard", args[0])
	}
}

func printUsage(w io.Writer) error {
	_, err := fmt.Fprint(w, `usage: conduit usage <command>

Commands:
  purge      delete entries (--before <date> | --model <name>) [--dry-run | --yes]
  export     write raw entries as csv or json (--format, --from, --to, --out)
  report     aggregate-only report (--group day|month|model|provider, --json)
  dashboard  render self-contained HTML report (--out)

All commands operate on ~/.conduit/logs/usage. No data leaves the machine.
`)
	return err
}

func defaultLogDirAbs() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("usage: resolve home dir: %w", err)
	}
	return filepath.Join(home, defaultLogDir), nil
}

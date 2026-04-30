package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const defaultTimeout = 5 * time.Second

// run launches command as a shell subprocess, pipes input as JSON to stdin,
// and parses the Output from stdout. On crash, timeout, or malformed output,
// it returns allow (fail-safe per PRD §6.6).
func run(ctx context.Context, command string, timeout time.Duration, input Input) Output {
	payload, err := json.Marshal(input)
	if err != nil {
		return Output{Decision: DecisionAllow}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", expandHome(command))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.Stdin = bytes.NewReader(payload)

	out, err := cmd.Output()
	if err != nil {
		// crash or timeout → fail-safe allow
		return Output{Decision: DecisionAllow}
	}

	var result Output
	if err := json.Unmarshal(bytes.TrimSpace(out), &result); err != nil {
		return Output{Decision: DecisionAllow}
	}
	if result.Decision == "" {
		result.Decision = DecisionAllow
	}
	return result
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return home + path[1:]
}

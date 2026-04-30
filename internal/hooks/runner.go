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
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Start(); err != nil {
		return Output{Decision: DecisionAllow}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return Output{Decision: DecisionAllow}
		}
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		<-done
		return Output{Decision: DecisionAllow}
	}

	var result Output
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
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

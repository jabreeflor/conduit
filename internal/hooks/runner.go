package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// DefaultTimeout is the hook subprocess timeout when the config omits one.
const DefaultTimeout = 5 * time.Second

// run executes one hook command as a shell subprocess using the JSON wire
// protocol. The input is written as JSON to stdin; the output is read as JSON
// from stdout. On any failure (crash, timeout, malformed JSON) the function
// returns a fail-safe "allow" decision so a broken hook never blocks the agent.
func run(ctx context.Context, command string, input HookInput) (HookOutput, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return HookOutput{Decision: DecisionAllow}, err
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = bytes.NewReader(inputJSON)
	// WaitDelay forces I/O goroutines to close after the context expires, so an
	// orphaned child holding stdout open cannot block us past the hook timeout.
	cmd.WaitDelay = 500 * time.Millisecond

	out, err := cmd.Output()
	if err != nil {
		// crash or timeout — fail-safe
		return HookOutput{Decision: DecisionAllow}, err
	}

	var output HookOutput
	if err := json.Unmarshal(bytes.TrimSpace(out), &output); err != nil || output.Decision == "" {
		return HookOutput{Decision: DecisionAllow}, err
	}
	return output, nil
}

// timeoutFor returns the effective timeout for a hook config entry.
func timeoutFor(h HookConfig) time.Duration {
	if h.Timeout > 0 {
		return time.Duration(h.Timeout) * time.Second
	}
	return DefaultTimeout
}

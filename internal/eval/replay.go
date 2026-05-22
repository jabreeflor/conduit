// Package eval runs Conduit's model evaluation workflows.
package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultSessionsDir = ".conduit/sessions"
	defaultResultsDir  = ".conduit/evals/results"
)

// Message is one replayable transcript event from a persisted session JSONL.
type Message struct {
	At      time.Time `json:"at,omitempty"`
	TurnID  string    `json:"turn_id,omitempty"`
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Model   string    `json:"model,omitempty"`
}

// ReplayRequest is the prompt and baseline output for one assistant turn.
type ReplayRequest struct {
	SessionID      string
	Model          string
	TurnIndex      int
	Prompt         string
	History        []Message
	ExpectedOutput string
}

// ReplayResponse is returned by the model adapter used for replay.
type ReplayResponse struct {
	Output       string `json:"output"`
	Provider     string `json:"provider,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
}

// Responder re-runs one assistant turn against a target model.
type Responder interface {
	Replay(context.Context, ReplayRequest) (ReplayResponse, error)
}

// SnapshotResponder is the offline fallback. It preserves the recorded
// assistant output so replay-as-eval can create a durable baseline before live
// provider execution is configured.
type SnapshotResponder struct{}

// Replay returns the recorded assistant output unchanged.
func (SnapshotResponder) Replay(_ context.Context, req ReplayRequest) (ReplayResponse, error) {
	return ReplayResponse{Output: req.ExpectedOutput, Provider: "snapshot"}, nil
}

// ReplayOptions controls one replay-as-eval run.
type ReplayOptions struct {
	SessionID   string
	Model       string
	Diff        bool
	SessionsDir string
	ResultsDir  string
	Responder   Responder
	Now         func() time.Time
}

// ReplayResult is one JSONL event written for every assistant turn.
type ReplayResult struct {
	At             time.Time `json:"at"`
	SessionID      string    `json:"session_id"`
	Model          string    `json:"model"`
	Provider       string    `json:"provider"`
	TurnIndex      int       `json:"turn_index"`
	TurnID         string    `json:"turn_id,omitempty"`
	Prompt         string    `json:"prompt"`
	ExpectedOutput string    `json:"expected_output"`
	ActualOutput   string    `json:"actual_output"`
	Matched        bool      `json:"matched"`
	InputTokens    int       `json:"input_tokens,omitempty"`
	OutputTokens   int       `json:"output_tokens,omitempty"`
}

// ReplaySummary is the user-facing outcome of a replay run.
type ReplaySummary struct {
	SessionID    string
	Model        string
	SourcePath   string
	ResultPath   string
	Turns        int
	Matches      int
	Diffs        []string
	SnapshotMode bool
}

// ReplaySession re-runs each recorded assistant turn and writes JSONL results.
func ReplaySession(ctx context.Context, opts ReplayOptions) (ReplaySummary, error) {
	if opts.SessionID == "" {
		return ReplaySummary{}, errors.New("eval replay requires a session id")
	}
	if opts.Model == "" {
		return ReplaySummary{}, errors.New("eval replay requires --model")
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.Responder == nil {
		opts.Responder = SnapshotResponder{}
	}

	sourcePath, err := ResolveSessionPath(opts.SessionID, opts.SessionsDir)
	if err != nil {
		return ReplaySummary{}, err
	}
	messages, err := ReadSession(sourcePath)
	if err != nil {
		return ReplaySummary{}, err
	}

	resultPath, err := resultPath(opts.SessionID, opts.Model, opts.ResultsDir, opts.Now())
	if err != nil {
		return ReplaySummary{}, err
	}
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		return ReplaySummary{}, fmt.Errorf("eval replay: create results dir: %w", err)
	}

	f, err := os.OpenFile(resultPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return ReplaySummary{}, fmt.Errorf("eval replay: open results: %w", err)
	}
	defer f.Close()

	summary := ReplaySummary{
		SessionID:    opts.SessionID,
		Model:        opts.Model,
		SourcePath:   sourcePath,
		ResultPath:   resultPath,
		SnapshotMode: isSnapshot(opts.Responder),
	}
	enc := json.NewEncoder(f)
	for i, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		history := append([]Message(nil), messages[:i]...)
		req := ReplayRequest{
			SessionID:      opts.SessionID,
			Model:          opts.Model,
			TurnIndex:      summary.Turns + 1,
			Prompt:         latestUserPrompt(history),
			History:        history,
			ExpectedOutput: msg.Content,
		}
		resp, err := opts.Responder.Replay(ctx, req)
		if err != nil {
			return summary, fmt.Errorf("eval replay: turn %d: %w", req.TurnIndex, err)
		}
		result := ReplayResult{
			At:             opts.Now(),
			SessionID:      opts.SessionID,
			Model:          opts.Model,
			Provider:       resp.Provider,
			TurnIndex:      req.TurnIndex,
			TurnID:         msg.TurnID,
			Prompt:         req.Prompt,
			ExpectedOutput: msg.Content,
			ActualOutput:   resp.Output,
			Matched:        strings.TrimSpace(msg.Content) == strings.TrimSpace(resp.Output),
			InputTokens:    resp.InputTokens,
			OutputTokens:   resp.OutputTokens,
		}
		if result.Provider == "" {
			result.Provider = "unknown"
		}
		if err := enc.Encode(result); err != nil {
			return summary, fmt.Errorf("eval replay: write result: %w", err)
		}
		summary.Turns++
		if result.Matched {
			summary.Matches++
		}
		if opts.Diff && !result.Matched {
			summary.Diffs = append(summary.Diffs, UnifiedDiff(
				fmt.Sprintf("session:%s:turn:%d:recorded", opts.SessionID, result.TurnIndex),
				fmt.Sprintf("session:%s:turn:%d:%s", opts.SessionID, result.TurnIndex, opts.Model),
				result.ExpectedOutput,
				result.ActualOutput,
			))
		}
	}
	if summary.Turns == 0 {
		return summary, errors.New("eval replay: session contains no assistant turns")
	}
	return summary, nil
}

// ResolveSessionPath finds a session JSONL file by id or direct path.
func ResolveSessionPath(sessionID, sessionsDir string) (string, error) {
	if sessionID == "" {
		return "", errors.New("session id is empty")
	}
	if info, err := os.Stat(sessionID); err == nil && !info.IsDir() {
		return sessionID, nil
	}
	if sessionsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("eval replay: resolve home dir: %w", err)
		}
		sessionsDir = filepath.Join(home, defaultSessionsDir)
	}
	candidates := []string{
		filepath.Join(sessionsDir, sessionID+".jsonl"),
		filepath.Join(sessionsDir, sessionID, "session.jsonl"),
		filepath.Join(sessionsDir, sessionID, "messages.jsonl"),
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("eval replay: session %q not found in %s", sessionID, sessionsDir)
}

// ReadSession parses replayable user and assistant messages from JSONL.
func ReadSession(path string) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("eval replay: open session: %w", err)
	}
	defer f.Close()
	return readSessionJSONL(f)
}

func readSessionJSONL(r io.Reader) ([]Message, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var messages []Message
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		msg, ok, err := parseMessageLine([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("eval replay: parse line %d: %w", lineNo, err)
		}
		if ok {
			messages = append(messages, msg)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("eval replay: scan session: %w", err)
	}
	if len(messages) == 0 {
		return nil, errors.New("eval replay: session contains no replayable messages")
	}
	return messages, nil
}

func parseMessageLine(line []byte) (Message, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return Message{}, false, err
	}
	role := firstString(raw, "role", "author", "speaker")
	if role == "" {
		role = normalizeEventType(firstString(raw, "type", "event"))
	}
	role = strings.ToLower(role)
	if role != "user" && role != "assistant" {
		return Message{}, false, nil
	}
	content := firstString(raw, "content", "text", "message", "output")
	if content == "" {
		return Message{}, false, nil
	}
	msg := Message{
		Role:    role,
		Content: content,
		TurnID:  firstString(raw, "turn_id", "turnId", "id"),
		Model:   firstString(raw, "model"),
	}
	if at := firstString(raw, "at", "time", "timestamp"); at != "" {
		msg.At, _ = time.Parse(time.RFC3339, at)
	}
	return msg, true, nil
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := raw[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			return t
		case map[string]any:
			if s := firstString(t, "content", "text"); s != "" {
				return s
			}
		}
	}
	return ""
}

func normalizeEventType(t string) string {
	switch strings.ToLower(t) {
	case "user_message", "message_user":
		return "user"
	case "assistant_message", "message_assistant", "model_output":
		return "assistant"
	default:
		return ""
	}
}

func latestUserPrompt(history []Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			return history[i].Content
		}
	}
	return ""
}

func resultPath(sessionID, model, resultsDir string, at time.Time) (string, error) {
	if resultsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("eval replay: resolve home dir: %w", err)
		}
		resultsDir = filepath.Join(home, defaultResultsDir)
	}
	name := fmt.Sprintf("%s-%s-%s.jsonl", slug(sessionID), slug(model), at.UTC().Format("20060102T150405Z"))
	return filepath.Join(resultsDir, name), nil
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func isSnapshot(r Responder) bool {
	_, ok := r.(SnapshotResponder)
	return ok
}

// UnifiedDiff renders a compact line-oriented diff for CLI output.
func UnifiedDiff(from, to, before, after string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", from)
	fmt.Fprintf(&b, "+++ %s\n", to)
	beforeLines := splitLines(before)
	afterLines := splitLines(after)
	max := len(beforeLines)
	if len(afterLines) > max {
		max = len(afterLines)
	}
	for i := 0; i < max; i++ {
		var left, right string
		if i < len(beforeLines) {
			left = beforeLines[i]
		}
		if i < len(afterLines) {
			right = afterLines[i]
		}
		switch {
		case i >= len(beforeLines):
			fmt.Fprintf(&b, "+%s\n", right)
		case i >= len(afterLines):
			fmt.Fprintf(&b, "-%s\n", left)
		case left == right:
			fmt.Fprintf(&b, " %s\n", left)
		default:
			fmt.Fprintf(&b, "-%s\n", left)
			fmt.Fprintf(&b, "+%s\n", right)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

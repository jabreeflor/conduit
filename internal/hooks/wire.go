package hooks

// DecisionType is the hook subprocess's verdict for a pending lifecycle event.
type DecisionType string

const (
	DecisionAllow  DecisionType = "allow"
	DecisionBlock  DecisionType = "block"
	DecisionInject DecisionType = "inject"
)

// HookInput is written as JSON to the hook subprocess stdin.
type HookInput struct {
	Event     string         `json:"event"`
	ToolName  string         `json:"tool_name,omitempty"`
	ToolInput map[string]any `json:"tool_input,omitempty"`
	SessionID string         `json:"session_id"`
	CWD       string         `json:"cwd"`
}

// HookOutput is read as JSON from the hook subprocess stdout.
type HookOutput struct {
	Decision DecisionType `json:"decision"`
	Reason   string       `json:"reason,omitempty"`
	Context  string       `json:"context,omitempty"` // payload for inject decisions
}

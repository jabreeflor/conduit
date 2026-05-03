package video

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Op is a single action the natural-language planner emits. Stored as
// data so callers can preview the plan, persist it for redo/undo, and
// apply it under their own ordering rules.
type Op struct {
	Kind   string // "trim" | "split" | "cut" | "transition" | "music" | "caption" | "speed" | "highlight" | "intro" | "outro"
	Params map[string]any
}

// Planner converts a user intent like "remove the ums and add an intro"
// into a sequence of Ops against an existing EDL.
type Planner struct {
	model Model
}

// Model is the LLM interface the planner needs.
type Model interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// NewPlanner constructs a Planner.
func NewPlanner(model Model) *Planner { return &Planner{model: model} }

// Plan returns the ordered list of Ops the model proposes.
//
// The plan is returned as data; the caller decides whether to apply it
// via Apply, present it for confirmation, or discard it. This keeps the
// natural-language surface non-destructive.
func (p *Planner) Plan(ctx context.Context, intent string, edl *EDL) ([]Op, error) {
	if p.model == nil {
		return nil, errors.New("video: nil model")
	}
	if strings.TrimSpace(intent) == "" {
		return nil, errors.New("video: empty intent")
	}
	system := planSystemPrompt()
	user := planUserPrompt(intent, edl)
	raw, err := p.model.Complete(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("video: model error: %w", err)
	}
	return parsePlan(raw)
}

// Apply runs the Ops sequentially against the EDL. Errors are returned
// from the first failing op; previously-applied ops are not rolled back
// — the caller is expected to snapshot the EDL beforehand.
func Apply(edl *EDL, ops []Op) error {
	for i, op := range ops {
		if err := applyOne(edl, op); err != nil {
			return fmt.Errorf("op %d (%s): %w", i, op.Kind, err)
		}
	}
	return nil
}

func applyOne(edl *EDL, op Op) error {
	switch op.Kind {
	case "trim":
		id, in, out, err := trimParams(op.Params)
		if err != nil {
			return err
		}
		return edl.Trim(id, in, out)
	case "split":
		id, at, err := splitParams(op.Params)
		if err != nil {
			return err
		}
		_, err = edl.Split(id, at)
		return err
	case "cut":
		start, end, err := rangeParams(op.Params)
		if err != nil {
			return err
		}
		edl.Cut(start, end)
		return nil
	case "transition":
		prev, next, kind, length, err := transitionParams(op.Params)
		if err != nil {
			return err
		}
		return edl.AddTransition(prev, next, kind, length)
	case "music":
		src, volume, duck, err := musicParams(op.Params)
		if err != nil {
			return err
		}
		edl.SetMusic(MusicTrack{Source: src, Volume: volume, AutoDuckdB: duck})
		return nil
	case "caption":
		start, end, text, err := captionParams(op.Params)
		if err != nil {
			return err
		}
		edl.AddCaption(Caption{Start: start, End: end, Text: text})
		return nil
	case "highlight":
		// Tag a marker; the export stage builds the reel.
		at, label, err := markerParams(op.Params)
		if err != nil {
			return err
		}
		edl.AddMarker(Marker{At: at, Label: label})
		return nil
	}
	return fmt.Errorf("unknown op kind %q", op.Kind)
}

func planSystemPrompt() string {
	return `You are the Conduit Video editing planner.
Given the user's intent and a JSON snapshot of the EDL, output a JSON array of Ops.
Each Op is {"kind": "...", "params": {...}}.
Recognized kinds: trim, split, cut, transition, music, caption, highlight.
Use timestamps in nanoseconds (Go time.Duration). Reply with the JSON array only.`
}

func planUserPrompt(intent string, edl *EDL) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Intent: %s\n", intent)
	fmt.Fprintf(&b, "EDL clip count: %d\n", len(edl.Clips))
	fmt.Fprintf(&b, "EDL duration: %s\n", edl.Duration())
	for _, c := range edl.Clips {
		fmt.Fprintf(&b, "- %s (%s) start=%s len=%s\n", c.ID, c.Kind, c.Start, c.Length())
	}
	return b.String()
}

// parsePlan accepts either a JSON array of ops or a fenced code block
// containing one. Errors carry the snippet so callers can debug bad
// model output.
func parsePlan(raw string) ([]Op, error) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	if s == "" {
		return nil, errors.New("video: empty plan")
	}
	ops, err := decodeOps(s)
	if err != nil {
		return nil, fmt.Errorf("video: parse plan: %w", err)
	}
	return ops, nil
}

// --- param helpers (each enforces required keys & types) --------------

func trimParams(p map[string]any) (string, Time, Time, error) {
	id, _ := p["id"].(string)
	in, errIn := dur(p, "in")
	out, errOut := dur(p, "out")
	if id == "" || errIn != nil || errOut != nil {
		return "", 0, 0, errors.New("trim requires id, in, out")
	}
	return id, in, out, nil
}

func splitParams(p map[string]any) (string, Time, error) {
	id, _ := p["id"].(string)
	at, err := dur(p, "at")
	if id == "" || err != nil {
		return "", 0, errors.New("split requires id, at")
	}
	return id, at, nil
}

func rangeParams(p map[string]any) (Time, Time, error) {
	start, errS := dur(p, "start")
	end, errE := dur(p, "end")
	if errS != nil || errE != nil {
		return 0, 0, errors.New("range requires start, end")
	}
	return start, end, nil
}

func transitionParams(p map[string]any) (string, string, string, Time, error) {
	prev, _ := p["prev"].(string)
	next, _ := p["next"].(string)
	kind, _ := p["kind"].(string)
	length, err := dur(p, "length")
	if prev == "" || next == "" || kind == "" || err != nil {
		return "", "", "", 0, errors.New("transition requires prev, next, kind, length")
	}
	return prev, next, kind, length, nil
}

func musicParams(p map[string]any) (string, float64, float64, error) {
	src, _ := p["source"].(string)
	if src == "" {
		return "", 0, 0, errors.New("music requires source")
	}
	vol, _ := p["volume"].(float64)
	if vol == 0 {
		vol = 0.5
	}
	duck, _ := p["duck_db"].(float64)
	if duck == 0 {
		duck = -12
	}
	return src, vol, duck, nil
}

func captionParams(p map[string]any) (Time, Time, string, error) {
	start, errS := dur(p, "start")
	end, errE := dur(p, "end")
	text, _ := p["text"].(string)
	if errS != nil || errE != nil || text == "" {
		return 0, 0, "", errors.New("caption requires start, end, text")
	}
	return start, end, text, nil
}

func markerParams(p map[string]any) (Time, string, error) {
	at, err := dur(p, "at")
	label, _ := p["label"].(string)
	if err != nil {
		return 0, "", err
	}
	return at, label, nil
}

func dur(p map[string]any, key string) (Time, error) {
	v, ok := p[key]
	if !ok {
		return 0, fmt.Errorf("missing %q", key)
	}
	switch n := v.(type) {
	case float64:
		return time.Duration(int64(n)), nil
	case int:
		return time.Duration(int64(n)), nil
	case int64:
		return time.Duration(n), nil
	case string:
		d, err := time.ParseDuration(n)
		if err != nil {
			return 0, err
		}
		return d, nil
	}
	return 0, fmt.Errorf("%q has unexpected type %T", key, v)
}

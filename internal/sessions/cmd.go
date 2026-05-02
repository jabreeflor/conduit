package sessions

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Dispatcher executes /sessions slash commands against a Store. The
// dispatcher is intentionally surface-agnostic: it returns text the
// caller writes to its own output (TUI, CLI, GUI log) and an optional
// LoadID the surface should switch into when the user requested a load.
type Dispatcher struct {
	Store     *Store
	Responder ReplayResponder // injected by surfaces; nil disables /sessions replay.
}

// Result is the structured outcome of a slash command. Output is the
// human-readable text the surface should render. LoadSessionID is set
// when the user asked to switch to a different session (via load,
// fork, or replay).
type Result struct {
	Output        string
	LoadSessionID string
}

// Dispatch parses the args (everything after `/sessions `) and routes
// to the right verb. The command string is split on whitespace; quoted
// arguments are not supported because none of the verbs need them.
func (d *Dispatcher) Dispatch(args []string) (Result, error) {
	if d == nil || d.Store == nil {
		return Result{}, errors.New("sessions: dispatcher not configured")
	}
	if len(args) == 0 {
		return Result{Output: helpText()}, nil
	}
	switch args[0] {
	case "list":
		return d.list()
	case "fork":
		return d.fork(args[1:])
	case "replay":
		return d.replay(args[1:])
	case "load":
		return d.load(args[1:])
	case "help", "-h", "--help":
		return Result{Output: helpText()}, nil
	default:
		return Result{}, fmt.Errorf("unknown /sessions subcommand %q; try: list, fork, replay, load", args[0])
	}
}

// DispatchLine is a convenience wrapper that accepts the raw input line
// (with or without the leading "/sessions" prefix) and splits it into
// args before dispatching.
func (d *Dispatcher) DispatchLine(line string) (Result, error) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "/sessions")
	line = strings.TrimSpace(line)
	if line == "" {
		return d.Dispatch(nil)
	}
	return d.Dispatch(strings.Fields(line))
}

// WriteResult is a small helper that writes r.Output followed by a
// trailing newline when the writer is non-nil and r has output to write.
func WriteResult(w io.Writer, r Result) {
	if w == nil || r.Output == "" {
		return
	}
	out := r.Output
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	io.WriteString(w, out)
}

func helpText() string {
	return strings.Join([]string{
		"/sessions list                        list known sessions",
		"/sessions fork <turn-id>              fork a new session from <turn-id>",
		"/sessions replay <turn-id> [--model M] re-issue assistant turn from <turn-id>",
		"/sessions load <session-id>           load <session-id> in the current surface",
	}, "\n")
}

func (d *Dispatcher) list() (Result, error) {
	infos, err := d.Store.List()
	if err != nil {
		return Result{}, err
	}
	if len(infos) == 0 {
		return Result{Output: "no sessions found in " + d.Store.Dir()}, nil
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTURNS\tFORKED FROM\tUPDATED\tTITLE")
	for _, info := range infos {
		forked := info.ForkParentID
		if forked == "" {
			forked = "-"
		}
		updated := "-"
		if !info.UpdatedAt.IsZero() {
			updated = info.UpdatedAt.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", info.ID, info.TurnCount, forked, updated, info.Title)
	}
	tw.Flush()
	return Result{Output: strings.TrimRight(b.String(), "\n")}, nil
}

func (d *Dispatcher) fork(args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, errors.New("usage: /sessions fork <turn-id>")
	}
	turnID := args[0]
	parentSess, _, err := d.Store.FindTurn(turnID)
	if err != nil {
		return Result{}, err
	}
	newSess, err := d.Store.Fork(parentSess.ID, turnID)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:        fmt.Sprintf("forked %s from turn %s -> new session %s", parentSess.ID, turnID, newSess.ID),
		LoadSessionID: newSess.ID,
	}, nil
}

func (d *Dispatcher) replay(args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, errors.New("usage: /sessions replay <turn-id> [--model M] [--key=val...]")
	}
	turnID := args[0]
	model, params, err := parseReplayFlags(args[1:])
	if err != nil {
		return Result{}, err
	}
	if d.Responder == nil {
		return Result{}, errors.New("sessions: replay disabled (no Responder injected)")
	}
	parentSess, _, err := d.Store.FindTurn(turnID)
	if err != nil {
		return Result{}, err
	}
	newSess, newTurn, err := d.Store.Replay(parentSess.ID, turnID, ReplayOptions{
		Model:     model,
		Params:    params,
		Responder: d.Responder,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output: fmt.Sprintf("replayed %s from turn %s with model %q -> session %s, new turn %s",
			parentSess.ID, turnID, model, newSess.ID, newTurn.ID),
		LoadSessionID: newSess.ID,
	}, nil
}

func (d *Dispatcher) load(args []string) (Result, error) {
	if len(args) < 1 {
		return Result{}, errors.New("usage: /sessions load <session-id>")
	}
	id := args[0]
	sess, err := d.Store.Load(id)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:        fmt.Sprintf("loaded session %s (%d turns)", sess.ID, len(sess.Turns)),
		LoadSessionID: sess.ID,
	}, nil
}

func parseReplayFlags(args []string) (model string, params map[string]string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--model":
			if i+1 >= len(args) {
				return "", nil, errors.New("--model requires a value")
			}
			i++
			model = args[i]
		case strings.HasPrefix(a, "--model="):
			model = strings.TrimPrefix(a, "--model=")
		case strings.HasPrefix(a, "--"):
			// Generic --key=value passthrough so callers can override
			// arbitrary inference parameters (temperature=0.2, etc.)
			// without us knowing the schema.
			body := strings.TrimPrefix(a, "--")
			eq := strings.IndexByte(body, '=')
			if eq < 0 {
				return "", nil, fmt.Errorf("flag %q must be --key=value", a)
			}
			if params == nil {
				params = map[string]string{}
			}
			params[body[:eq]] = body[eq+1:]
		default:
			return "", nil, fmt.Errorf("unexpected positional argument %q", a)
		}
	}
	return model, params, nil
}

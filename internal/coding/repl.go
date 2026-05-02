package coding

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/replytags"
	"github.com/jabreeflor/conduit/internal/tools"
)

// Streamer is the provider-facing contract the REPL uses to emit one
// assistant turn. The REPL invokes Stream once per turn; the implementation
// owns chunking, backpressure, and finishReason semantics. Real provider
// clients land in a follow-up — for now the cmd wiring injects a stub.
type Streamer interface {
	Stream(ctx context.Context, prompt string, onDelta func(string)) (full string, finishReason string, err error)
}

// Continuer decides whether a truncated assistant turn should auto-resume.
// Default impl returns true on "length" / "max_tokens"; surfaces can swap
// in stricter logic (e.g. cap on auto-continuations per session).
type Continuer interface {
	ShouldContinue(finishReason string) bool
}

// DefaultContinuer is the standard truncation-aware continuer.
type DefaultContinuer struct{}

// ShouldContinue resumes when the provider signals it ran out of output
// space, not when it stopped naturally. "length" is the OpenAI convention,
// "max_tokens" is the Anthropic convention; we accept both so the REPL is
// provider-agnostic.
func (DefaultContinuer) ShouldContinue(finishReason string) bool {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "length", "max_tokens":
		return true
	default:
		return false
	}
}

// REPL is the multi-turn coding loop. It owns no provider client and no
// real tool runners — both are dependencies. The intent is that integration
// tests and follow-up PRs can swap pieces individually without rebuilding
// the whole loop.
type REPL struct {
	Session   *Session
	Budget    *Budget
	Tools     []tools.Tool
	Streamer  Streamer
	Continuer Continuer
	In        io.Reader
	Out       io.Writer

	// MaxAutoContinue caps how many follow-up "continue" turns the REPL will
	// auto-issue per user message. 4 is a pragmatic ceiling that handles
	// long structured responses without enabling runaway loops on a
	// misbehaving provider.
	MaxAutoContinue int

	// TagSink, if non-nil, receives structured reply tags ([[silent]],
	// [[voice: ...]], [[media: ...]], [[heartbeat]], [[canvas: ...]]) parsed
	// out of the assistant stream. Tag bytes are filtered from Out; only
	// plain text reaches the user. The session journal still records the
	// full unfiltered turn so replays can re-dispatch tags. Leave nil to
	// keep the legacy passthrough behavior. See PRD §6.14.
	TagSink func(replytags.Event)
}

// Run drives the read/stream/append loop until the input is exhausted or
// the context is cancelled. Both terminate cleanly with nil error.
func (r *REPL) Run(ctx context.Context) error {
	if r.Session == nil {
		return errors.New("coding repl: session required")
	}
	if r.Streamer == nil {
		return errors.New("coding repl: streamer required")
	}
	if r.Continuer == nil {
		r.Continuer = DefaultContinuer{}
	}
	if r.MaxAutoContinue <= 0 {
		r.MaxAutoContinue = 4
	}
	if r.In == nil {
		r.In = strings.NewReader("")
	}
	if r.Out == nil {
		r.Out = io.Discard
	}

	scanner := bufio.NewScanner(r.In)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fmt.Fprint(r.Out, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}
		line := scanner.Text()
		if line == "" {
			continue
		}

		userTurn := contracts.CodingTurn{Role: "user", Content: line}
		if _, err := r.Session.Append(userTurn); err != nil {
			return err
		}

		if err := r.streamAssistant(ctx, line, false); err != nil {
			return err
		}
	}
}

// streamAssistant runs one provider call and recursively auto-continues if
// the finish reason is truncation-shaped and the budget still has room.
// `isContinuation` only changes the prompt the streamer sees so providers
// that need a "continue" nudge get one.
func (r *REPL) streamAssistant(ctx context.Context, prompt string, isContinuation bool) error {
	autoCount := 0
	current := prompt
	for {
		var assistantBuf strings.Builder

		// When a TagSink is wired in, deltas flow through the reply-tag
		// parser so [[silent]] / [[voice: ...]] / etc. never reach the
		// terminal verbatim. The raw stream is still buffered into
		// assistantBuf for the session journal.
		var tagParser *replytags.Parser
		var silenced bool
		if r.TagSink != nil {
			tagParser = replytags.New(replytags.Sink{
				Text: func(s string) {
					if !silenced {
						fmt.Fprint(r.Out, s)
					}
				},
				Tag: func(ev replytags.Event) {
					if ev.Kind == replytags.KindSilent {
						silenced = true
					}
					r.TagSink(ev)
				},
			})
		}

		full, finish, err := r.Streamer.Stream(ctx, current, func(delta string) {
			assistantBuf.WriteString(delta)
			if tagParser != nil {
				_, _ = tagParser.Write(delta)
				return
			}
			fmt.Fprint(r.Out, delta)
		})
		if tagParser != nil {
			tagParser.Close()
		}
		if err != nil {
			return err
		}
		if full == "" {
			full = assistantBuf.String()
		}
		fmt.Fprintln(r.Out)

		if _, err := r.Session.Append(contracts.CodingTurn{Role: "assistant", Content: full}); err != nil {
			return err
		}

		// Token observation: rough char/4 estimator until a real tokenizer
		// is wired in alongside the provider client. Marked as a placeholder
		// so the next PR knows to replace it before any cost-sensitive
		// routing depends on it.
		if r.Budget != nil {
			r.Budget.Observe(estimateTokens(current), estimateTokens(full))
		}

		if !r.Continuer.ShouldContinue(finish) {
			return nil
		}
		if r.Budget != nil && r.Budget.ShouldCompact() {
			// Don't stack more truncated output onto a budget that already
			// needs compaction; the next PR's compactor handles this turn.
			return nil
		}
		autoCount++
		if autoCount >= r.MaxAutoContinue {
			return nil
		}
		_ = isContinuation
		current = "continue"
	}
}

// estimateTokens is a placeholder char/4 heuristic. A real tokenizer keyed
// on the active model lands with the provider streamer.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return len(s) / 4
}

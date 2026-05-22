// Package replytags parses structured reply tags emitted by agents and
// dispatches them to surface-specific handlers (TTS, Canvas, GUI media).
// See PRD §6.14 for the tag protocol.
//
// Supported tags:
//
//	[[silent]]         — suppress visible output for this turn
//	[[voice: text]]    — speak `text` via TTS
//	[[media: url]]     — render media at `url` inline
//	[[heartbeat]]      — keep-alive ping for long-running operations
//	[[canvas: html]]   — render `html` to the GUI Canvas panel
//
// The parser is streaming-tolerant: a tag may arrive split across an
// arbitrary number of Write calls (e.g. "[[voi" then "ce: hi]]"). Plain
// text outside of tags is forwarded to the Sink's Text handler verbatim.
// Unrecognized or malformed tags are passed through as plain text so a
// stray "[[foo]]" never disappears silently.
package replytags

import (
	"strings"
)

// Kind identifies one of the five recognized tag types.
type Kind string

const (
	KindSilent    Kind = "silent"
	KindVoice     Kind = "voice"
	KindMedia     Kind = "media"
	KindHeartbeat Kind = "heartbeat"
	KindCanvas    Kind = "canvas"
)

// Event is a structured tag emission. Body is empty for marker tags
// (silent, heartbeat) and carries the trimmed payload for content tags
// (voice, media, canvas).
type Event struct {
	Kind Kind
	Body string
}

// Sink receives parser output. Text is invoked for plain text outside any
// tag; Tag is invoked for each recognized tag. Either field may be nil —
// nil handlers drop their input, which is how callers implement
// suppression (e.g. set Text=nil after observing [[silent]]).
type Sink struct {
	Text func(string)
	Tag  func(Event)
}

// openDelim and closeDelim are the literal tag boundaries. Kept as
// constants (not regex) so the streaming state machine stays cheap and
// allocation-free on the hot delta path.
const (
	openDelim  = "[["
	closeDelim = "]]"
)

// Parser is a streaming tag extractor. Feed it deltas via Write; flush
// any trailing buffered text with Close. A Parser is single-use and not
// safe for concurrent Write calls — callers serialize delta delivery
// (which the REPL already does).
type Parser struct {
	sink Sink

	// buf holds bytes that may belong to either an in-progress tag or a
	// trailing partial open delimiter. The parser only emits text once
	// it can prove the bytes are not part of a tag.
	buf strings.Builder

	// inTag tracks whether we have observed an open delimiter and are
	// accumulating the tag body in buf (which excludes the delimiter
	// itself once inTag flips true).
	inTag bool
}

// New returns a Parser that dispatches to sink.
func New(sink Sink) *Parser {
	return &Parser{sink: sink}
}

// Write feeds a chunk of streamed assistant output through the parser.
// It always returns len(s), nil — the io.Writer signature is for
// convenience when wiring into existing pipelines.
func (p *Parser) Write(s string) (int, error) {
	n := len(s)
	if s == "" {
		return 0, nil
	}
	p.buf.WriteString(s)
	p.drain()
	return n, nil
}

// Close flushes any buffered bytes. An unclosed tag (e.g. "[[voice: hi"
// with no terminating "]]") is emitted as plain text so the user sees
// what the agent actually said instead of losing the tail of the turn.
func (p *Parser) Close() {
	if p.inTag {
		// Re-prepend the open delimiter that was stripped when inTag
		// flipped, so the user sees the literal source text.
		p.emitText(openDelim + p.buf.String())
		p.buf.Reset()
		p.inTag = false
		return
	}
	if p.buf.Len() > 0 {
		p.emitText(p.buf.String())
		p.buf.Reset()
	}
}

// drain consumes as much of the buffer as can be unambiguously
// classified, leaving only bytes that might still be part of an
// in-progress tag (a trailing "[" or partial "[[...]" body).
func (p *Parser) drain() {
	for {
		current := p.buf.String()
		if current == "" {
			return
		}

		if !p.inTag {
			// Hunt for the next potential open delimiter.
			idx := strings.Index(current, openDelim)
			if idx < 0 {
				// No "[[" anywhere. Hold back a single trailing "["
				// because the next chunk might complete the delimiter.
				if strings.HasSuffix(current, "[") {
					if len(current) > 1 {
						p.emitText(current[:len(current)-1])
					}
					p.buf.Reset()
					p.buf.WriteByte('[')
					return
				}
				p.emitText(current)
				p.buf.Reset()
				return
			}
			if idx > 0 {
				p.emitText(current[:idx])
			}
			// Skip the "[[" delimiter itself; remaining buffer is the
			// tag body candidate.
			p.buf.Reset()
			p.buf.WriteString(current[idx+len(openDelim):])
			p.inTag = true
			continue
		}

		// inTag: scan for the close delimiter.
		end := strings.Index(current, closeDelim)
		if end < 0 {
			// Need more bytes. If the buffer is getting long without a
			// close, that's still tolerated — the agent might be
			// emitting a large [[canvas:...]] payload.
			return
		}
		body := current[:end]
		rest := current[end+len(closeDelim):]
		p.dispatchTag(body)
		p.inTag = false
		p.buf.Reset()
		p.buf.WriteString(rest)
	}
}

// dispatchTag classifies the body of a closed [[...]] tag and emits the
// appropriate Event. Unknown tags pass through as plain text wrapped in
// the original delimiters so the user never silently loses content.
func (p *Parser) dispatchTag(body string) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		// "[[]]" — almost certainly not intended as a tag. Echo it.
		p.emitText(openDelim + body + closeDelim)
		return
	}

	// Marker tags: exact (case-insensitive) match, no payload allowed.
	switch strings.ToLower(trimmed) {
	case "silent":
		p.emitTag(Event{Kind: KindSilent})
		return
	case "heartbeat":
		p.emitTag(Event{Kind: KindHeartbeat})
		return
	}

	// Content tags: "<name>:<payload>". Split on the first colon so
	// payloads (URLs, HTML, prose) can contain colons freely.
	colon := strings.IndexByte(trimmed, ':')
	if colon < 0 {
		p.emitText(openDelim + body + closeDelim)
		return
	}
	name := strings.ToLower(strings.TrimSpace(trimmed[:colon]))
	payload := strings.TrimSpace(trimmed[colon+1:])
	switch name {
	case "voice":
		p.emitTag(Event{Kind: KindVoice, Body: payload})
	case "media":
		p.emitTag(Event{Kind: KindMedia, Body: payload})
	case "canvas":
		p.emitTag(Event{Kind: KindCanvas, Body: payload})
	default:
		p.emitText(openDelim + body + closeDelim)
	}
}

func (p *Parser) emitText(s string) {
	if s == "" || p.sink.Text == nil {
		return
	}
	p.sink.Text(s)
}

func (p *Parser) emitTag(ev Event) {
	if p.sink.Tag == nil {
		return
	}
	p.sink.Tag(ev)
}

// Parse is a one-shot helper for non-streaming callers (eval assertions,
// hook payloads, tests). It runs Write+Close once and returns the
// collected events plus the plain-text remainder.
func Parse(input string) (text string, events []Event) {
	var sb strings.Builder
	p := New(Sink{
		Text: func(s string) { sb.WriteString(s) },
		Tag:  func(ev Event) { events = append(events, ev) },
	})
	_, _ = p.Write(input)
	p.Close()
	return sb.String(), events
}

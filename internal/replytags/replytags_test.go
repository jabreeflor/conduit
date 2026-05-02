package replytags_test

import (
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/replytags"
)

func TestParse_PlainText(t *testing.T) {
	text, events := replytags.Parse("hello world")
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
	if len(events) != 0 {
		t.Errorf("events = %v, want none", events)
	}
}

func TestParse_SilentTag(t *testing.T) {
	text, events := replytags.Parse("[[silent]]")
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(events) != 1 || events[0].Kind != replytags.KindSilent {
		t.Fatalf("events = %v, want one silent event", events)
	}
	if events[0].Body != "" {
		t.Errorf("silent body = %q, want empty", events[0].Body)
	}
}

func TestParse_HeartbeatTag(t *testing.T) {
	text, events := replytags.Parse("working...[[heartbeat]] still here")
	if text != "working... still here" {
		t.Errorf("text = %q", text)
	}
	if len(events) != 1 || events[0].Kind != replytags.KindHeartbeat {
		t.Fatalf("events = %v, want one heartbeat", events)
	}
}

func TestParse_VoiceTag(t *testing.T) {
	text, events := replytags.Parse("[[voice: hello there]]")
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(events) != 1 {
		t.Fatalf("events = %v, want one", events)
	}
	if events[0].Kind != replytags.KindVoice || events[0].Body != "hello there" {
		t.Errorf("event = %+v", events[0])
	}
}

func TestParse_MediaTag(t *testing.T) {
	text, events := replytags.Parse("see [[media: https://example.com/x.png]] !")
	if text != "see  !" {
		t.Errorf("text = %q", text)
	}
	if len(events) != 1 || events[0].Kind != replytags.KindMedia ||
		events[0].Body != "https://example.com/x.png" {
		t.Errorf("event = %+v", events[0])
	}
}

func TestParse_CanvasTag(t *testing.T) {
	html := "<h1>Hello</h1><p>colon-friendly: yes</p>"
	text, events := replytags.Parse("[[canvas: " + html + "]]done")
	if text != "done" {
		t.Errorf("text = %q", text)
	}
	if len(events) != 1 || events[0].Kind != replytags.KindCanvas ||
		events[0].Body != html {
		t.Errorf("event = %+v", events[0])
	}
}

func TestParse_MixedTagsAndText(t *testing.T) {
	in := "Pre [[heartbeat]] mid [[voice: speak]] post [[media: u]] end"
	text, events := replytags.Parse(in)

	wantText := "Pre  mid  post  end"
	if text != wantText {
		t.Errorf("text = %q, want %q", text, wantText)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	wantKinds := []replytags.Kind{
		replytags.KindHeartbeat, replytags.KindVoice, replytags.KindMedia,
	}
	for i, k := range wantKinds {
		if events[i].Kind != k {
			t.Errorf("events[%d].Kind = %s, want %s", i, events[i].Kind, k)
		}
	}
	if events[1].Body != "speak" {
		t.Errorf("voice body = %q", events[1].Body)
	}
	if events[2].Body != "u" {
		t.Errorf("media body = %q", events[2].Body)
	}
}

func TestParse_UnclosedTagPassesThrough(t *testing.T) {
	in := "okay [[voice: never closed"
	text, events := replytags.Parse(in)
	if text != in {
		t.Errorf("text = %q, want original %q", text, in)
	}
	if len(events) != 0 {
		t.Errorf("events = %v, want none", events)
	}
}

func TestParse_UnknownTagPassesThrough(t *testing.T) {
	in := "[[bogus]] and [[also: thing]]"
	text, events := replytags.Parse(in)
	if text != in {
		t.Errorf("text = %q, want original %q", text, in)
	}
	if len(events) != 0 {
		t.Errorf("events = %v, want none", events)
	}
}

func TestParse_EmptyTagPassesThrough(t *testing.T) {
	text, events := replytags.Parse("[[]]")
	if text != "[[]]" {
		t.Errorf("text = %q, want literal", text)
	}
	if len(events) != 0 {
		t.Errorf("events = %v, want none", events)
	}
}

func TestParse_TagNameIsCaseInsensitive(t *testing.T) {
	text, events := replytags.Parse("[[Silent]] [[VOICE: hi]] [[Canvas: x]]")
	if text != "  " {
		t.Errorf("text = %q", text)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Kind != replytags.KindSilent ||
		events[1].Kind != replytags.KindVoice || events[1].Body != "hi" ||
		events[2].Kind != replytags.KindCanvas || events[2].Body != "x" {
		t.Errorf("events = %+v", events)
	}
}

func TestParse_LoneOpenBracketsArePlainText(t *testing.T) {
	text, events := replytags.Parse("a [ b [c d")
	if text != "a [ b [c d" {
		t.Errorf("text = %q", text)
	}
	if len(events) != 0 {
		t.Errorf("events = %v, want none", events)
	}
}

func TestStreaming_TagAcrossManyChunks(t *testing.T) {
	chunks := []string{"hello ", "[", "[", "voi", "ce", ": stre", "amed]", "]", " bye"}

	var textBuf strings.Builder
	var events []replytags.Event
	p := replytags.New(replytags.Sink{
		Text: func(s string) { textBuf.WriteString(s) },
		Tag:  func(ev replytags.Event) { events = append(events, ev) },
	})
	for _, c := range chunks {
		if _, err := p.Write(c); err != nil {
			t.Fatalf("Write(%q): %v", c, err)
		}
	}
	p.Close()

	if textBuf.String() != "hello  bye" {
		t.Errorf("text = %q, want %q", textBuf.String(), "hello  bye")
	}
	if len(events) != 1 || events[0].Kind != replytags.KindVoice ||
		events[0].Body != "streamed" {
		t.Errorf("events = %+v", events)
	}
}

func TestStreaming_TextIsForwardedIncrementally(t *testing.T) {
	// Plain text without any "[" must flush eagerly so the surface can
	// render tokens as they arrive. We assert by counting Text calls.
	var calls int
	p := replytags.New(replytags.Sink{
		Text: func(string) { calls++ },
	})
	for _, c := range []string{"a", "b", "c"} {
		_, _ = p.Write(c)
	}
	p.Close()
	if calls != 3 {
		t.Errorf("Text invocations = %d, want 3 (one per chunk)", calls)
	}
}

func TestStreaming_TrailingOpenBracketHeldUntilFlush(t *testing.T) {
	// A trailing "[" might be the start of "[[". The parser must hold it
	// back until either the next chunk arrives or Close is called.
	var sb strings.Builder
	p := replytags.New(replytags.Sink{
		Text: func(s string) { sb.WriteString(s) },
	})
	_, _ = p.Write("hello[")
	if sb.String() != "hello" {
		t.Errorf("after partial: %q, want %q", sb.String(), "hello")
	}
	p.Close()
	if sb.String() != "hello[" {
		t.Errorf("after close: %q, want %q", sb.String(), "hello[")
	}
}

func TestStreaming_AdjacentTags(t *testing.T) {
	text, events := replytags.Parse("[[silent]][[heartbeat]][[voice: x]]")
	if text != "" {
		t.Errorf("text = %q, want empty", text)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	if events[0].Kind != replytags.KindSilent ||
		events[1].Kind != replytags.KindHeartbeat ||
		events[2].Kind != replytags.KindVoice || events[2].Body != "x" {
		t.Errorf("events = %+v", events)
	}
}

func TestSink_NilHandlersDropSilently(t *testing.T) {
	// Verify a Sink with nil Text and nil Tag is valid (callers use this
	// to implement [[silent]] suppression).
	p := replytags.New(replytags.Sink{})
	if _, err := p.Write("text [[heartbeat]] more"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p.Close()
}

package sessions

import (
	"strings"
	"testing"
)

func newDispatcherWithSession(t *testing.T) (*Dispatcher, *Session, Turn) {
	t.Helper()
	store := newTestStore(t)
	sess, _ := store.Create()
	a, _ := store.Append(sess, Turn{Role: "user", Content: "hello"})
	store.Append(sess, Turn{Role: "assistant", Content: "world", ParentID: a.ID})
	return &Dispatcher{Store: store, Responder: &stubResponder{out: "stub"}}, sess, a
}

func TestDispatchHelpDefault(t *testing.T) {
	d, _, _ := newDispatcherWithSession(t)
	res, err := d.Dispatch(nil)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(res.Output, "/sessions list") {
		t.Errorf("help missing list verb: %q", res.Output)
	}
}

func TestDispatchListIncludesSession(t *testing.T) {
	d, sess, _ := newDispatcherWithSession(t)
	res, err := d.Dispatch([]string{"list"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(res.Output, sess.ID) {
		t.Errorf("list output missing session id %q: %q", sess.ID, res.Output)
	}
}

func TestDispatchForkProducesNewSession(t *testing.T) {
	d, sess, a := newDispatcherWithSession(t)
	res, err := d.Dispatch([]string{"fork", a.ID})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.LoadSessionID == "" {
		t.Error("fork should set LoadSessionID for the surface to switch to")
	}
	if res.LoadSessionID == sess.ID {
		t.Error("fork must point at a new session id")
	}
	if !strings.Contains(res.Output, "forked") {
		t.Errorf("fork output should describe what happened: %q", res.Output)
	}
}

func TestDispatchReplayUsesResponderAndModelOverride(t *testing.T) {
	d, _, a := newDispatcherWithSession(t)
	res, err := d.Dispatch([]string{"replay", a.ID, "--model", "claude-opus-4-6", "--temperature=0.2"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.LoadSessionID == "" {
		t.Error("replay should set LoadSessionID")
	}
	if !strings.Contains(res.Output, "claude-opus-4-6") {
		t.Errorf("replay output should mention chosen model: %q", res.Output)
	}
}

func TestDispatchReplayWithoutResponderErrors(t *testing.T) {
	d, _, a := newDispatcherWithSession(t)
	d.Responder = nil
	if _, err := d.Dispatch([]string{"replay", a.ID, "--model", "x"}); err == nil {
		t.Fatal("expected error when Responder is nil")
	}
}

func TestDispatchLoadSwitchesSession(t *testing.T) {
	d, sess, _ := newDispatcherWithSession(t)
	res, err := d.Dispatch([]string{"load", sess.ID})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.LoadSessionID != sess.ID {
		t.Errorf("LoadSessionID: got %q want %q", res.LoadSessionID, sess.ID)
	}
}

func TestDispatchUnknownVerbErrors(t *testing.T) {
	d, _, _ := newDispatcherWithSession(t)
	if _, err := d.Dispatch([]string{"explode"}); err == nil {
		t.Fatal("expected error for unknown verb")
	}
}

func TestDispatchLineStripsPrefix(t *testing.T) {
	d, sess, _ := newDispatcherWithSession(t)
	res, err := d.DispatchLine("/sessions load " + sess.ID)
	if err != nil {
		t.Fatalf("DispatchLine: %v", err)
	}
	if res.LoadSessionID != sess.ID {
		t.Errorf("LoadSessionID: got %q want %q", res.LoadSessionID, sess.ID)
	}
}

func TestDispatchForkUnknownTurnErrors(t *testing.T) {
	d, _, _ := newDispatcherWithSession(t)
	if _, err := d.Dispatch([]string{"fork", "no-such-turn"}); err == nil {
		t.Fatal("expected error for unknown turn id")
	}
}

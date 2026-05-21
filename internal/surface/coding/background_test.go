package coding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type bgFakeStreamer struct {
	out    string
	finish string
	err    error
	delay  time.Duration
}

func (f bgFakeStreamer) Stream(ctx context.Context, _ string, onDelta func(string)) (string, string, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", "", f.err
	}
	if onDelta != nil {
		onDelta(f.out)
	}
	return f.out, f.finish, nil
}

func TestBackgroundStartAndComplete(t *testing.T) {
	m, err := NewBackgroundManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	job, err := m.Start(context.Background(), BackgroundStartOptions{
		Prompt:   "hello",
		Streamer: bgFakeStreamer{out: "world", finish: "stop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Wait(job.ID); err != nil {
		t.Fatal(err)
	}
	got, err := m.Get(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != BackgroundCompleted {
		t.Errorf("expected completed, got %s (err=%s)", got.State, got.Error)
	}
	logs, err := m.ReadLog(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logs), "world") {
		t.Errorf("log missing streamed content: %s", logs)
	}
}

func TestBackgroundFailedReportsError(t *testing.T) {
	m, _ := NewBackgroundManager(t.TempDir())
	job, err := m.Start(context.Background(), BackgroundStartOptions{
		Prompt:   "x",
		Streamer: bgFakeStreamer{err: errors.New("provider boom")},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = m.Wait(job.ID)
	got, _ := m.Get(job.ID)
	if got.State != BackgroundFailed || !strings.Contains(got.Error, "provider boom") {
		t.Errorf("expected failed: %+v", got)
	}
}

func TestBackgroundKill(t *testing.T) {
	m, _ := NewBackgroundManager(t.TempDir())
	job, err := m.Start(context.Background(), BackgroundStartOptions{
		Prompt:   "long",
		Streamer: bgFakeStreamer{out: "ok", finish: "stop", delay: 5 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Kill(job.ID); err != nil {
		t.Fatal(err)
	}
	_ = m.Wait(job.ID)
	got, _ := m.Get(job.ID)
	if got.State != BackgroundKilled {
		t.Errorf("expected killed, got %s", got.State)
	}
}

func TestBackgroundListAndPersist(t *testing.T) {
	home := t.TempDir()
	m, _ := NewBackgroundManager(home)
	for i := 0; i < 3; i++ {
		_, err := m.Start(context.Background(), BackgroundStartOptions{
			Prompt:   "p",
			Streamer: bgFakeStreamer{out: "ok", finish: "stop"},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, j := range m.List() {
		_ = m.Wait(j.ID)
	}
	if got := m.List(); len(got) != 3 {
		t.Errorf("expected 3 in-process jobs, got %d", len(got))
	}

	// Fresh manager rooted at the same home should see the persisted index.
	m2, _ := NewBackgroundManager(home)
	got, err := m2.ListPersisted()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 persisted jobs, got %d", len(got))
	}
	for _, j := range got {
		if j.State != BackgroundCompleted {
			t.Errorf("persisted job state: %+v", j)
		}
	}
}

func TestBackgroundStartValidation(t *testing.T) {
	m, _ := NewBackgroundManager(t.TempDir())
	if _, err := m.Start(context.Background(), BackgroundStartOptions{Streamer: bgFakeStreamer{}}); err == nil {
		t.Errorf("expected prompt-required error")
	}
	if _, err := m.Start(context.Background(), BackgroundStartOptions{Prompt: "p"}); err == nil {
		t.Errorf("expected streamer-required error")
	}
}

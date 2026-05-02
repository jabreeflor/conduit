package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileCheckpointerWriteThenRead(t *testing.T) {
	dir := t.TempDir()
	cp, err := NewFileCheckpointer(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointer: %v", err)
	}

	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	original := &Run{
		ID:         "run-1",
		WorkflowID: "wf-greet",
		Workflow: Workflow{
			ID:   "wf-greet",
			Name: "greet",
			Steps: []Step{
				{ID: "s1", Name: "first", Prompt: "hello"},
				{ID: "s2", Name: "second", Prompt: "world"},
			},
			Providers: []string{"primary", "secondary"},
		},
		State:       RunStateRunning,
		CurrentStep: 1,
		Results: []StepResult{
			{
				StepID:      "s1",
				Provider:    "primary",
				Output:      "hi",
				StartedAt:   now,
				CompletedAt: now.Add(time.Second),
			},
		},
		CreatedAt: now,
		UpdatedAt: now.Add(time.Second),
	}

	if err := cp.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File should land at the canonical path.
	if _, err := os.Stat(filepath.Join(dir, "run-1.json")); err != nil {
		t.Fatalf("expected checkpoint file: %v", err)
	}

	loaded, err := cp.Load("run-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != original.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, original.ID)
	}
	if loaded.State != original.State {
		t.Errorf("State = %q, want %q", loaded.State, original.State)
	}
	if loaded.CurrentStep != original.CurrentStep {
		t.Errorf("CurrentStep = %d, want %d", loaded.CurrentStep, original.CurrentStep)
	}
	if len(loaded.Workflow.Steps) != len(original.Workflow.Steps) {
		t.Fatalf("Steps len = %d, want %d", len(loaded.Workflow.Steps), len(original.Workflow.Steps))
	}
	if len(loaded.Workflow.Providers) != 2 || loaded.Workflow.Providers[0] != "primary" {
		t.Errorf("Providers roundtrip mismatch: %+v", loaded.Workflow.Providers)
	}
	if len(loaded.Results) != 1 {
		t.Fatalf("Results len = %d, want 1", len(loaded.Results))
	}
	if got, _ := loaded.Results[0].Output.(string); got != "hi" {
		t.Errorf("Results[0].Output = %v, want %q", loaded.Results[0].Output, "hi")
	}
	if !loaded.UpdatedAt.Equal(original.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", loaded.UpdatedAt, original.UpdatedAt)
	}
}

func TestFileCheckpointerLoadMissing(t *testing.T) {
	cp, err := NewFileCheckpointer(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCheckpointer: %v", err)
	}
	_, err = cp.Load("does-not-exist")
	if !errors.Is(err, ErrCheckpointNotFound) {
		t.Fatalf("expected ErrCheckpointNotFound, got %v", err)
	}
}

func TestFileCheckpointerOverwriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	cp, err := NewFileCheckpointer(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointer: %v", err)
	}

	run := &Run{ID: "run-x", WorkflowID: "wf"}
	run.State = RunStatePending
	if err := cp.Save(run); err != nil {
		t.Fatalf("Save 1: %v", err)
	}

	run.State = RunStateCompleted
	run.CurrentStep = 7
	if err := cp.Save(run); err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	loaded, err := cp.Load("run-x")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.State != RunStateCompleted || loaded.CurrentStep != 7 {
		t.Errorf("overwrite did not take effect: %+v", loaded)
	}

	// No leftover .tmp files in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", entry.Name())
		}
	}
}

func TestFileCheckpointerRequiresDir(t *testing.T) {
	if _, err := NewFileCheckpointer(""); err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestFileCheckpointerRejectsBadInput(t *testing.T) {
	cp, err := NewFileCheckpointer(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCheckpointer: %v", err)
	}
	if err := cp.Save(nil); err == nil {
		t.Fatal("Save(nil) should fail")
	}
	if err := cp.Save(&Run{}); err == nil {
		t.Fatal("Save without ID should fail")
	}
	if _, err := cp.Load(""); err == nil {
		t.Fatal("Load(\"\") should fail")
	}
}

func TestDefaultCheckpointDirShape(t *testing.T) {
	dir, err := DefaultCheckpointDir()
	if err != nil {
		t.Fatalf("DefaultCheckpointDir: %v", err)
	}
	if filepath.Base(dir) != "runs" || filepath.Base(filepath.Dir(dir)) != ".conduit" {
		t.Errorf("unexpected default dir shape: %s", dir)
	}
}

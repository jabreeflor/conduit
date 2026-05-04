package coding

import (
	"errors"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// RecordModelCall
// ---------------------------------------------------------------------------

func TestSessionBudget_RecordModelCall_NoLimit(t *testing.T) {
	b := NewSessionBudget(SessionLimits{})
	for i := 0; i < 100; i++ {
		if err := b.RecordModelCall(); err != nil {
			t.Fatalf("unexpected error at call %d: %v", i, err)
		}
	}
	snap := b.Snapshot()
	if snap.ModelCallCount != 100 {
		t.Errorf("want 100 model calls, got %d", snap.ModelCallCount)
	}
}

func TestSessionBudget_RecordModelCall_ExceedsLimit(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxModelCalls: ptr(2)})
	if err := b.RecordModelCall(); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordModelCall(); err != nil {
		t.Fatal(err)
	}
	// Third call should fail (limit is 2, count is already 2).
	err := b.RecordModelCall()
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RecordToolCall
// ---------------------------------------------------------------------------

func TestSessionBudget_RecordToolCall_NoLimit(t *testing.T) {
	b := NewSessionBudget(SessionLimits{})
	for i := 0; i < 50; i++ {
		if err := b.RecordToolCall(); err != nil {
			t.Fatalf("unexpected error at call %d: %v", i, err)
		}
	}
}

func TestSessionBudget_RecordToolCall_ExceedsLimit(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxToolCalls: ptr(1)})
	if err := b.RecordToolCall(); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordToolCall(); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RecordTurn
// ---------------------------------------------------------------------------

func TestSessionBudget_RecordTurn_ExceedsLimit(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxSessionTurns: ptr(3)})
	for i := 0; i < 3; i++ {
		if err := b.RecordTurn(); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}
	if err := b.RecordTurn(); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RecordDelegatedTask
// ---------------------------------------------------------------------------

func TestSessionBudget_RecordDelegatedTask_ExceedsLimit(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxDelegatedTasks: ptr(0)})
	// MaxDelegatedTasks=0 means zero tasks allowed.
	// IntPtr(0) returns nil (no limit), so test with explicit ptr(0).
	if err := b.RecordDelegatedTask(); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded for limit=0, got %v", err)
	}
}

func TestSessionBudget_RecordDelegatedTask_WithLimit(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxDelegatedTasks: ptr(2)})
	if err := b.RecordDelegatedTask(); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordDelegatedTask(); err != nil {
		t.Fatal(err)
	}
	if err := b.RecordDelegatedTask(); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RecordTokens — per-call limits
// ---------------------------------------------------------------------------

func TestSessionBudget_RecordTokens_InputPerCallExceeded(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxInputTokensPerCall: ptr(100)})
	err := b.RecordTokens(101, 10, 0, 0)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

func TestSessionBudget_RecordTokens_OutputPerCallExceeded(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxOutputTokensPerCall: ptr(50)})
	err := b.RecordTokens(10, 51, 0, 0)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

func TestSessionBudget_RecordTokens_ReasoningPerCallExceeded(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxReasoningTokensPerCall: ptr(20)})
	err := b.RecordTokens(10, 10, 21, 0)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// RecordTokens — cumulative limits
// ---------------------------------------------------------------------------

func TestSessionBudget_RecordTokens_TotalTokensExceeded(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxTotalTokens: ptr(100)})
	// First call: 60 in + 30 out = 90 total — OK.
	if err := b.RecordTokens(60, 30, 0, 0); err != nil {
		t.Fatal(err)
	}
	// Second call: 10 in + 5 out pushes total to 105 — should fail.
	err := b.RecordTokens(10, 5, 0, 0)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

func TestSessionBudget_RecordTokens_BudgetUSDExceeded(t *testing.T) {
	b := NewSessionBudget(SessionLimits{MaxBudgetUSD: ptrFloat(0.01)})
	if err := b.RecordTokens(0, 0, 0, 0.005); err != nil {
		t.Fatal(err)
	}
	// Cumulative cost: 0.005 + 0.006 = 0.011 > 0.01 → should fail.
	err := b.RecordTokens(0, 0, 0, 0.006)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("want ErrBudgetExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

func TestSessionBudget_Snapshot_AccumulatesCorrectly(t *testing.T) {
	b := NewSessionBudget(SessionLimits{})
	_ = b.RecordModelCall()
	_ = b.RecordModelCall()
	_ = b.RecordToolCall()
	_ = b.RecordTokens(100, 50, 10, 0.002)
	_ = b.RecordTokens(200, 80, 0, 0.003)
	_ = b.RecordTurn()

	snap := b.Snapshot()
	if snap.ModelCallCount != 2 {
		t.Errorf("want 2 model calls, got %d", snap.ModelCallCount)
	}
	if snap.ToolCallCount != 1 {
		t.Errorf("want 1 tool call, got %d", snap.ToolCallCount)
	}
	if snap.TotalInputTokens != 300 {
		t.Errorf("want 300 input tokens, got %d", snap.TotalInputTokens)
	}
	if snap.TotalOutputTokens != 130 {
		t.Errorf("want 130 output tokens, got %d", snap.TotalOutputTokens)
	}
	if snap.TotalReasoningTokens != 10 {
		t.Errorf("want 10 reasoning tokens, got %d", snap.TotalReasoningTokens)
	}
	const wantCost = 0.005
	if snap.EstimatedCostUSD < wantCost-1e-9 || snap.EstimatedCostUSD > wantCost+1e-9 {
		t.Errorf("want cost %.4f, got %.4f", wantCost, snap.EstimatedCostUSD)
	}
	if snap.SessionTurnCount != 1 {
		t.Errorf("want 1 turn, got %d", snap.SessionTurnCount)
	}
}

// ---------------------------------------------------------------------------
// intPtr / float64Ptr helpers
// ---------------------------------------------------------------------------

func TestIntPtr_ZeroReturnsNil(t *testing.T) {
	if IntPtr(0) != nil {
		t.Error("IntPtr(0) should return nil")
	}
}

func TestIntPtr_NegativeReturnsNil(t *testing.T) {
	if IntPtr(-5) != nil {
		t.Error("IntPtr(-5) should return nil")
	}
}

func TestIntPtr_PositiveReturnsPointer(t *testing.T) {
	p := IntPtr(42)
	if p == nil {
		t.Fatal("IntPtr(42) should return non-nil")
	}
	if *p != 42 {
		t.Errorf("want 42, got %d", *p)
	}
}

func TestFloat64Ptr_ZeroReturnsNil(t *testing.T) {
	if Float64Ptr(0) != nil {
		t.Error("Float64Ptr(0) should return nil")
	}
}

func TestFloat64Ptr_PositiveReturnsPointer(t *testing.T) {
	p := Float64Ptr(3.14)
	if p == nil {
		t.Fatal("Float64Ptr(3.14) should return non-nil")
	}
	if *p != 3.14 {
		t.Errorf("want 3.14, got %f", *p)
	}
}

// ptrFloat is a local helper for float64 pointer creation in tests.
func ptrFloat(v float64) *float64 { return &v }

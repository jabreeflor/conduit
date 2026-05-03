package gui

import (
	"testing"
)

// helpers -----------------------------------------------------------------

func makeNode(id, label string) DAGNode {
	return DAGNode{
		ID:     id,
		Label:  label,
		Status: NodeStatusPending,
		Inputs: map[string]any{"prompt": "do " + id},
	}
}

// -------------------------------------------------------------------------

func TestWorkflowDAG_AddAndNode(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Fetch data"))

	n := d.Node("step-1")
	if n == nil {
		t.Fatal("Node() returned nil for inserted node")
	}
	if n.ID != "step-1" {
		t.Errorf("ID = %q, want step-1", n.ID)
	}
	if n.Label != "Fetch data" {
		t.Errorf("Label = %q, want Fetch data", n.Label)
	}
	if n.Status != NodeStatusPending {
		t.Errorf("Status = %v, want NodeStatusPending", n.Status)
	}
}

func TestWorkflowDAG_NodeMissing(t *testing.T) {
	d := NewWorkflowDAG()
	if d.Node("nonexistent") != nil {
		t.Error("Node() should return nil for unknown ID")
	}
}

func TestWorkflowDAG_Len(t *testing.T) {
	d := NewWorkflowDAG()
	if d.Len() != 0 {
		t.Errorf("empty DAG Len = %d, want 0", d.Len())
	}
	d.AddNode(makeNode("a", "A"))
	d.AddNode(makeNode("b", "B"))
	if d.Len() != 2 {
		t.Errorf("Len = %d, want 2", d.Len())
	}
}

func TestWorkflowDAG_AddNodePreservesOrder(t *testing.T) {
	d := NewWorkflowDAG()
	ids := []string{"step-1", "step-2", "step-3"}
	for _, id := range ids {
		d.AddNode(makeNode(id, id))
	}

	nodes := d.Nodes()
	if len(nodes) != 3 {
		t.Fatalf("Nodes() len = %d, want 3", len(nodes))
	}
	for i, n := range nodes {
		if n.ID != ids[i] {
			t.Errorf("nodes[%d].ID = %q, want %q", i, n.ID, ids[i])
		}
	}
}

func TestWorkflowDAG_AddNodeReplacesExisting(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Old label"))

	updated := makeNode("step-1", "New label")
	updated.Status = NodeStatusRunning
	d.AddNode(updated)

	// Insertion order: still 1 entry.
	if d.Len() != 1 {
		t.Errorf("Len = %d after replace, want 1", d.Len())
	}
	n := d.Node("step-1")
	if n.Label != "New label" {
		t.Errorf("Label = %q, want New label", n.Label)
	}
	if n.Status != NodeStatusRunning {
		t.Errorf("Status = %v, want NodeStatusRunning", n.Status)
	}
}

func TestWorkflowDAG_UpdateStatus(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))

	d.UpdateStatus("step-1", NodeStatusRunning)
	if got := d.Node("step-1").Status; got != NodeStatusRunning {
		t.Errorf("Status = %v, want NodeStatusRunning", got)
	}

	d.UpdateStatus("step-1", NodeStatusCompleted)
	if got := d.Node("step-1").Status; got != NodeStatusCompleted {
		t.Errorf("Status = %v, want NodeStatusCompleted", got)
	}
}

func TestWorkflowDAG_UpdateStatusNoopOnMissing(t *testing.T) {
	d := NewWorkflowDAG()
	// Should not panic.
	d.UpdateStatus("nonexistent", NodeStatusRunning)
}

func TestWorkflowDAG_SetOutputSuccess(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))
	d.UpdateStatus("step-1", NodeStatusRunning)

	d.SetOutput("step-1", map[string]any{"result": "ok"}, "")

	n := d.Node("step-1")
	if n.Status != NodeStatusCompleted {
		t.Errorf("Status = %v, want NodeStatusCompleted", n.Status)
	}
	if n.Error != "" {
		t.Errorf("Error = %q, want empty", n.Error)
	}
	out, ok := n.Output.(map[string]any)
	if !ok {
		t.Fatalf("Output type = %T, want map[string]any", n.Output)
	}
	if out["result"] != "ok" {
		t.Errorf("Output[result] = %v, want ok", out["result"])
	}
}

func TestWorkflowDAG_SetOutputFailure(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))
	d.UpdateStatus("step-1", NodeStatusRunning)

	d.SetOutput("step-1", nil, "timeout after 30s")

	n := d.Node("step-1")
	if n.Status != NodeStatusFailed {
		t.Errorf("Status = %v, want NodeStatusFailed", n.Status)
	}
	if n.Error != "timeout after 30s" {
		t.Errorf("Error = %q, want timeout after 30s", n.Error)
	}
}

func TestWorkflowDAG_SetOutputNoopOnMissing(t *testing.T) {
	d := NewWorkflowDAG()
	// Should not panic.
	d.SetOutput("nonexistent", "value", "")
}

func TestWorkflowDAG_SelectAndInspect(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))
	d.SetOutput("step-1", "hello", "")

	if d.SelectedNode() != nil {
		t.Error("SelectedNode should be nil before any selection")
	}

	d.SelectNode("step-1")
	sel := d.SelectedNode()
	if sel == nil {
		t.Fatal("SelectedNode() returned nil after SelectNode")
	}
	if sel.ID != "step-1" {
		t.Errorf("SelectedNode.ID = %q, want step-1", sel.ID)
	}
	if sel.Output != "hello" {
		t.Errorf("SelectedNode.Output = %v, want hello", sel.Output)
	}
}

func TestWorkflowDAG_SelectNodeNoopOnMissing(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))
	d.SelectNode("step-1")

	// Selecting a non-existent node does not overwrite current selection.
	d.SelectNode("nonexistent")
	if sel := d.SelectedNode(); sel == nil || sel.ID != "step-1" {
		t.Error("SelectNode on missing ID should not clear current selection")
	}
}

func TestWorkflowDAG_DeselectNode(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))
	d.SelectNode("step-1")

	d.DeselectNode()
	if d.SelectedNode() != nil {
		t.Error("SelectedNode should be nil after DeselectNode")
	}
}

func TestWorkflowDAG_ActiveNode(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))
	d.AddNode(makeNode("step-2", "Step 2"))

	if d.ActiveNode() != nil {
		t.Error("ActiveNode should be nil when no node is running")
	}

	d.UpdateStatus("step-2", NodeStatusRunning)
	active := d.ActiveNode()
	if active == nil {
		t.Fatal("ActiveNode() returned nil when step-2 is running")
	}
	if active.ID != "step-2" {
		t.Errorf("ActiveNode.ID = %q, want step-2", active.ID)
	}
}

func TestWorkflowDAG_ActiveNodeFirstInInsertionOrder(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("a", "A"))
	d.AddNode(makeNode("b", "B"))
	d.UpdateStatus("a", NodeStatusRunning)
	d.UpdateStatus("b", NodeStatusRunning)

	if active := d.ActiveNode(); active.ID != "a" {
		t.Errorf("ActiveNode.ID = %q, want a (first in insertion order)", active.ID)
	}
}

func TestWorkflowDAG_ShowHideVisible(t *testing.T) {
	d := NewWorkflowDAG()
	if d.Visible() {
		t.Error("new DAG should not be visible")
	}

	d.Show()
	if !d.Visible() {
		t.Error("expected visible after Show()")
	}

	d.Hide()
	if d.Visible() {
		t.Error("expected hidden after Hide()")
	}
}

func TestWorkflowDAG_Clear(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))
	d.AddNode(makeNode("step-2", "Step 2"))
	d.SelectNode("step-1")
	d.Show()

	d.Clear()

	if d.Len() != 0 {
		t.Errorf("Len = %d after Clear, want 0", d.Len())
	}
	if d.Visible() {
		t.Error("DAG should be hidden after Clear")
	}
	if d.SelectedNode() != nil {
		t.Error("SelectedNode should be nil after Clear")
	}
	if d.ActiveNode() != nil {
		t.Error("ActiveNode should be nil after Clear")
	}
}

func TestWorkflowDAG_NodesReturnsCopies(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Step 1"))

	nodes := d.Nodes()
	// Mutating the returned slice must not affect internal state.
	nodes[0].Label = "mutated"
	if got := d.Node("step-1").Label; got != "Step 1" {
		t.Errorf("Nodes() returned reference, not copy: Label = %q", got)
	}
}

func TestWorkflowDAG_EdgesStoredWithNode(t *testing.T) {
	d := NewWorkflowDAG()
	n := makeNode("step-1", "Step 1")
	n.Edges = []string{"step-2", "step-3"}
	d.AddNode(n)

	got := d.Node("step-1")
	if len(got.Edges) != 2 || got.Edges[0] != "step-2" || got.Edges[1] != "step-3" {
		t.Errorf("Edges = %v, want [step-2 step-3]", got.Edges)
	}
}

func TestWorkflowDAG_ConcurrentAccess(t *testing.T) {
	d := NewWorkflowDAG()
	done := make(chan struct{})

	go func() {
		for i := 0; i < 50; i++ {
			d.AddNode(makeNode("a", "A"))
			d.UpdateStatus("a", NodeStatusRunning)
			d.SetOutput("a", nil, "")
		}
		done <- struct{}{}
	}()

	for i := 0; i < 50; i++ {
		_ = d.Node("a")
		_ = d.ActiveNode()
		_ = d.Nodes()
	}
	<-done
	// No panic = concurrent safety confirmed.
}

func TestWorkflowDAG_EmptyDAGNodesSnapshot(t *testing.T) {
	d := NewWorkflowDAG()
	nodes := d.Nodes()
	if nodes == nil {
		t.Error("Nodes() on empty DAG should return non-nil empty slice")
	}
	if len(nodes) != 0 {
		t.Errorf("Nodes() on empty DAG returned %d nodes, want 0", len(nodes))
	}
}

func TestWorkflowDAG_StatusTransitionLifecycle(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Process"))

	// Full happy-path lifecycle.
	d.UpdateStatus("step-1", NodeStatusRunning)
	if d.Node("step-1").Status != NodeStatusRunning {
		t.Error("expected NodeStatusRunning")
	}
	d.SetOutput("step-1", "done", "")
	if d.Node("step-1").Status != NodeStatusCompleted {
		t.Error("expected NodeStatusCompleted after successful SetOutput")
	}
}

func TestWorkflowDAG_SkippedStatus(t *testing.T) {
	d := NewWorkflowDAG()
	d.AddNode(makeNode("step-1", "Conditional step"))
	d.UpdateStatus("step-1", NodeStatusSkipped)

	if got := d.Node("step-1").Status; got != NodeStatusSkipped {
		t.Errorf("Status = %v, want NodeStatusSkipped", got)
	}
}

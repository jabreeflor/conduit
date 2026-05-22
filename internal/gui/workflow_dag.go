package gui

import "sync"

// NodeStatus is the visual render state of a workflow DAG node.
// The rendering layer maps each value to a distinct appearance:
// pending = dimmed outline, running = pulsing, completed = solid,
// failed = solid red, skipped = muted. See PRD §11.2.
type NodeStatus int

const (
	// NodeStatusPending means the step has not yet executed.
	NodeStatusPending NodeStatus = iota
	// NodeStatusRunning means the step is currently executing; the UI renders
	// it as a pulsing rounded rectangle.
	NodeStatusRunning
	// NodeStatusCompleted means the step finished successfully; rendered solid.
	NodeStatusCompleted
	// NodeStatusFailed means the step terminated with an error; rendered red.
	NodeStatusFailed
	// NodeStatusSkipped means the step was bypassed (e.g. its `if` condition
	// evaluated to false); rendered muted.
	NodeStatusSkipped
)

// DAGNode is a single step in the workflow DAG view-model.
//
// Inputs and Output hold the step's runtime values so the inspection panel
// can surface them when the user clicks the node. Both fields are nil until
// the execution engine calls WorkflowDAG.SetOutput.
type DAGNode struct {
	// ID is the step's stable identifier, matching WorkflowStep.ID.
	ID string
	// Label is the human-readable name displayed inside the node rectangle.
	Label string
	// Status is the current visual render state.
	Status NodeStatus
	// Edges holds the IDs of direct successor nodes (for DAG edge rendering).
	Edges []string
	// Inputs is the structured argument map passed to this step at execution
	// time. Set by AddNode or updated by SetInputs.
	Inputs map[string]any
	// Output is the value produced by this step. Nil until the step completes
	// or fails. May be any JSON-serialisable type.
	Output any
	// Error is the human-readable failure reason. Non-empty only when
	// Status == NodeStatusFailed.
	Error string
}

// WorkflowDAG is the view-model for the workflow DAG panel shown in the GUI
// main content area. It stores an ordered set of nodes and their directed
// edges, tracks per-node execution status for the rendering layer (pulsing,
// solid, red), and records which node the user last clicked so an inspection
// panel can display that node's inputs and outputs.
//
// WorkflowDAG is safe for concurrent use: the workflow engine and the UI event
// loop run on different goroutines.
type WorkflowDAG struct {
	mu sync.RWMutex

	nodes        map[string]*DAGNode // keyed by DAGNode.ID
	order        []string            // node IDs in insertion order
	selectedNode string              // ID of the inspected node, or ""
	visible      bool                // whether this panel is the active main content view
}

// NewWorkflowDAG returns an empty, hidden workflow DAG panel.
func NewWorkflowDAG() *WorkflowDAG {
	return &WorkflowDAG{
		nodes: make(map[string]*DAGNode),
	}
}

// AddNode inserts or replaces a node. If a node with the same ID already
// exists its fields are replaced entirely, preserving insertion order.
// The panel does not become visible automatically — call Show() when the
// workflow starts.
func (d *WorkflowDAG) AddNode(node DAGNode) {
	d.mu.Lock()
	defer d.mu.Unlock()

	cp := node
	if _, exists := d.nodes[node.ID]; !exists {
		d.order = append(d.order, node.ID)
	}
	d.nodes[node.ID] = &cp
}

// UpdateStatus sets the status of an existing node. If the node does not
// exist the call is a no-op.
func (d *WorkflowDAG) UpdateStatus(id string, status NodeStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if n, ok := d.nodes[id]; ok {
		n.Status = status
	}
}

// SetOutput records the result of a completed or failed step. output is the
// structured value produced by the step (may be nil); errMsg is the failure
// reason (empty on success). SetOutput also transitions the node status to
// NodeStatusCompleted when errMsg is empty and NodeStatusFailed otherwise.
// No-op if the node does not exist.
func (d *WorkflowDAG) SetOutput(id string, output any, errMsg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	n, ok := d.nodes[id]
	if !ok {
		return
	}
	n.Output = output
	n.Error = errMsg
	if errMsg != "" {
		n.Status = NodeStatusFailed
	} else {
		n.Status = NodeStatusCompleted
	}
}

// SelectNode marks id as the node currently open in the inspection panel.
// Selecting a node that does not exist is a no-op. Call DeselectNode to clear
// the selection.
func (d *WorkflowDAG) SelectNode(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.nodes[id]; ok {
		d.selectedNode = id
	}
}

// DeselectNode clears the inspection selection.
func (d *WorkflowDAG) DeselectNode() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.selectedNode = ""
}

// SelectedNode returns a copy of the currently selected node, or nil if no
// node is selected.
func (d *WorkflowDAG) SelectedNode() *DAGNode {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.selectedNode == "" {
		return nil
	}
	if n, ok := d.nodes[d.selectedNode]; ok {
		cp := *n
		return &cp
	}
	return nil
}

// Node returns a copy of the node with the given ID, or nil if it does not
// exist.
func (d *WorkflowDAG) Node(id string) *DAGNode {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if n, ok := d.nodes[id]; ok {
		cp := *n
		return &cp
	}
	return nil
}

// ActiveNode returns a copy of the node currently in NodeStatusRunning, or nil
// if no node is running. When multiple nodes have NodeStatusRunning (parallel
// execution), the first one in insertion order is returned.
func (d *WorkflowDAG) ActiveNode() *DAGNode {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, id := range d.order {
		n := d.nodes[id]
		if n.Status == NodeStatusRunning {
			cp := *n
			return &cp
		}
	}
	return nil
}

// Nodes returns a snapshot of all nodes in insertion order.
func (d *WorkflowDAG) Nodes() []DAGNode {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]DAGNode, 0, len(d.order))
	for _, id := range d.order {
		out = append(out, *d.nodes[id])
	}
	return out
}

// Len returns the number of nodes in the DAG.
func (d *WorkflowDAG) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.order)
}

// Visible reports whether the workflow DAG panel is the active main content view.
func (d *WorkflowDAG) Visible() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.visible
}

// Show makes the workflow DAG panel the active main content view.
func (d *WorkflowDAG) Show() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.visible = true
}

// Hide deactivates the workflow DAG panel, returning the main content area to
// the previous view (screenshot stream, canvas, etc.).
func (d *WorkflowDAG) Hide() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.visible = false
}

// Clear removes all nodes, clears the selection, and hides the panel.
func (d *WorkflowDAG) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.nodes = make(map[string]*DAGNode)
	d.order = nil
	d.selectedNode = ""
	d.visible = false
}

package sessions

import (
	"testing"
)

func TestBuildTreeFlatSession(t *testing.T) {
	store := newTestStore(t)
	sess, _ := store.Create()
	a, _ := store.Append(sess, Turn{Role: "user", Content: "one"})
	b, _ := store.Append(sess, Turn{Role: "assistant", Content: "two", ParentID: a.ID})

	tree, err := BuildTree(store)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if len(tree.Roots) != 1 {
		t.Fatalf("expected one root session, got %d", len(tree.Roots))
	}
	root := tree.Roots[0]
	if root.Kind != NodeKindSession {
		t.Errorf("root kind: got %v", root.Kind)
	}
	if len(root.Children) != 1 {
		t.Fatalf("session should have one direct child (turn a); got %d", len(root.Children))
	}
	if root.Children[0].Turn.ID != a.ID {
		t.Errorf("first turn under session should be %q; got %q", a.ID, root.Children[0].Turn.ID)
	}
	if len(root.Children[0].Children) != 1 || root.Children[0].Children[0].Turn.ID != b.ID {
		t.Errorf("turn b should hang off turn a")
	}
	if got := tree.LookupTurn(b.ID); got == nil || got.Turn.ID != b.ID {
		t.Errorf("LookupTurn(b) returned %v", got)
	}
}

func TestBuildTreeForkAttachesAcrossSessions(t *testing.T) {
	store := newTestStore(t)
	src, _ := store.Create()
	a, _ := store.Append(src, Turn{Role: "user", Content: "a"})
	b, _ := store.Append(src, Turn{Role: "assistant", Content: "b", ParentID: a.ID})
	if _, err := store.Fork(src.ID, b.ID); err != nil {
		t.Fatalf("Fork: %v", err)
	}

	tree, err := BuildTree(store)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	// Only the source session should be a root; the fork hangs off turn b.
	if len(tree.Roots) != 1 {
		t.Fatalf("expected one root, got %d", len(tree.Roots))
	}
	parentNode := tree.LookupTurn(b.ID)
	if parentNode == nil {
		t.Fatal("turn b not indexed")
	}
	foundFork := false
	for _, child := range parentNode.Children {
		if child.Kind == NodeKindTurn {
			// First fork turn should reference b as parent.
			if child.Turn.ParentID == b.ID && child.Turn.SessionID != src.ID {
				foundFork = true
				break
			}
		}
	}
	if !foundFork {
		t.Errorf("fork turn not attached under turn %q; children=%+v", b.ID, parentNode.Children)
	}
}

func TestTreeWalkVisitsAllNodes(t *testing.T) {
	store := newTestStore(t)
	sess, _ := store.Create()
	a, _ := store.Append(sess, Turn{Role: "user", Content: "a"})
	store.Append(sess, Turn{Role: "assistant", Content: "b", ParentID: a.ID})

	tree, _ := BuildTree(store)
	count := 0
	tree.Walk(func(n *Node, depth int) { count++ })
	if count < 3 { // session header + 2 turns
		t.Errorf("expected >=3 nodes visited, got %d", count)
	}
}

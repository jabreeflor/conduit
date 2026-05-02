package sessions

import (
	"sort"
)

// Node is one entry in the materialised session tree. A node may be
// either a Turn (Kind=NodeKindTurn, Turn populated) or a Session header
// (Kind=NodeKindSession, Session populated). Sessions appear as roots
// when they have no fork parent; otherwise they hang off the turn they
// were forked from.
type Node struct {
	Kind     NodeKind
	Turn     Turn         // populated when Kind == NodeKindTurn
	Session  *SessionInfo // populated when Kind == NodeKindSession
	Children []*Node
}

// NodeKind identifies whether a Node represents a Turn or a Session
// header.
type NodeKind int

const (
	// NodeKindTurn marks a turn-level node.
	NodeKindTurn NodeKind = iota
	// NodeKindSession marks a session-level node (a JSONL file).
	NodeKindSession
)

// Tree is the materialised forest of all sessions in a Store. Roots are
// sessions that were not forked; each session contains a chain of turn
// nodes; forks attach as session children to the turn they branched from.
type Tree struct {
	Roots []*Node
	// turnIndex maps turn id -> node so callers can navigate by id without
	// re-walking the tree.
	turnIndex map[string]*Node
}

// LookupTurn returns the node for the given turn id, or nil if absent.
func (t *Tree) LookupTurn(id string) *Node {
	if t == nil {
		return nil
	}
	return t.turnIndex[id]
}

// BuildTree assembles the full session forest. Each session expands into
// a chain of turn nodes; sessions whose first turn has a ParentID hang
// off that parent turn as a child Session node. Turns missing parents
// (orphans) attach to the session header.
func BuildTree(store *Store) (*Tree, error) {
	infos, err := store.List()
	if err != nil {
		return nil, err
	}
	tree := &Tree{turnIndex: map[string]*Node{}}
	sessionNodes := map[string]*Node{}

	// First pass: build per-session subtrees of turns.
	loaded := make(map[string]*Session, len(infos))
	for _, info := range infos {
		sess, err := store.Load(info.ID)
		if err != nil {
			continue
		}
		loaded[sess.ID] = sess
		// Snapshot the info loop variable so taking its address is safe.
		info := info
		root := &Node{Kind: NodeKindSession, Session: &info}
		sessionNodes[sess.ID] = root

		// Index turns by id and attach them to their parent within the
		// same session. Turns whose parent lives in another session
		// (fork roots) attach to the session node as the in-session root.
		nodes := map[string]*Node{}
		for _, t := range sess.Turns {
			t := t
			n := &Node{Kind: NodeKindTurn, Turn: t}
			nodes[t.ID] = n
			tree.turnIndex[t.ID] = n
		}
		for _, t := range sess.Turns {
			n := nodes[t.ID]
			if t.ParentID == "" {
				root.Children = append(root.Children, n)
				continue
			}
			parent, ok := nodes[t.ParentID]
			if ok {
				parent.Children = append(parent.Children, n)
			} else {
				// Parent is in another session — defer; we'll attach in
				// pass 2 once every session's turn index exists.
				root.Children = append(root.Children, n)
			}
		}
	}

	// Second pass: re-parent fork-root turns onto their cross-session
	// parents so the global tree shows the fork relationship.
	for _, sess := range loaded {
		root := sessionNodes[sess.ID]
		if root == nil || len(sess.Turns) == 0 {
			continue
		}
		first := sess.Turns[0]
		if first.ParentID == "" {
			continue
		}
		// If the parent turn lives in a different session, transplant
		// the in-session root chain onto that turn.
		parentNode := tree.turnIndex[first.ParentID]
		if parentNode == nil {
			continue
		}
		// Detach from session header and reattach under parent turn.
		var keep []*Node
		var moved []*Node
		for _, child := range root.Children {
			if child.Kind == NodeKindTurn && child.Turn.ID == first.ID {
				moved = append(moved, child)
			} else {
				keep = append(keep, child)
			}
		}
		root.Children = keep
		parentNode.Children = append(parentNode.Children, moved...)
	}

	// Identify roots: sessions with no fork parent.
	for id, sess := range loaded {
		if sess.ForkParentID == "" {
			tree.Roots = append(tree.Roots, sessionNodes[id])
		}
	}
	sort.Slice(tree.Roots, func(i, j int) bool {
		ai := tree.Roots[i].Session.UpdatedAt
		bi := tree.Roots[j].Session.UpdatedAt
		return ai.After(bi)
	})
	return tree, nil
}

// Walk pre-order traverses the tree and invokes fn for each node with
// its depth (root sessions are depth 0).
func (t *Tree) Walk(fn func(node *Node, depth int)) {
	if t == nil {
		return
	}
	var visit func(*Node, int)
	visit = func(n *Node, depth int) {
		fn(n, depth)
		for _, c := range n.Children {
			visit(c, depth+1)
		}
	}
	for _, r := range t.Roots {
		visit(r, 0)
	}
}

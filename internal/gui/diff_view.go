// Package gui — diff visualization view-model (issue #69).
//
// Every file write/edit performed by the agent emits a DiffEntry. The
// DiffView holds the live list, supports side-by-side and unified rendering,
// hunk-level approve/reject, and per-line annotations that flow back to the
// agent (PRD §6.19, §11.3).
package gui

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// DiffMode controls how a hunk is rendered.
type DiffMode int

const (
	DiffModeUnified    DiffMode = iota // single column, +/- prefixes
	DiffModeSideBySide                 // two columns: before | after
)

// LineKind classifies a single line in a diff hunk.
type LineKind int

const (
	LineContext LineKind = iota // unchanged context line
	LineAdded                   // present only in the new file
	LineRemoved                 // present only in the old file
)

// DiffLine is one row in a hunk.
type DiffLine struct {
	Kind    LineKind
	OldLine int    // 1-based old-file line number; 0 for added lines
	NewLine int    // 1-based new-file line number; 0 for removed lines
	Text    string // raw text without trailing newline
}

// HunkStatus tracks user disposition for a single hunk.
type HunkStatus int

const (
	HunkPending HunkStatus = iota
	HunkApproved
	HunkRejected
)

// Hunk is a contiguous set of changed lines plus surrounding context.
type Hunk struct {
	ID       string // stable identifier within the parent DiffEntry
	OldStart int    // 1-based starting line in the old file
	NewStart int    // 1-based starting line in the new file
	Lines    []DiffLine
	Status   HunkStatus
}

// Annotation is feedback the user attached to a specific line; it is
// surfaced to the agent as additional context on the next turn.
type Annotation struct {
	HunkID  string
	LineIdx int    // index into Hunk.Lines
	Body    string // free-form user comment
	At      time.Time
}

// DiffEntry represents one file write/edit by the agent.
type DiffEntry struct {
	ID          string // stable identifier (e.g. file path + sequence)
	Path        string // file path relative to the workspace root
	BaseSHA     string // git blob SHA of the base ("" for new files / pre-session)
	Tracked     bool   // true when the file is tracked in git
	CreatedAt   time.Time
	hunks       []*Hunk
	annotations []Annotation
}

// NewDiffEntry constructs a diff entry. Hunks may be appended afterwards.
func NewDiffEntry(id, path string, tracked bool, baseSHA string) *DiffEntry {
	return &DiffEntry{
		ID:        id,
		Path:      path,
		BaseSHA:   baseSHA,
		Tracked:   tracked,
		CreatedAt: time.Now(),
	}
}

// AppendHunk adds a hunk to the entry. Hunks are appended in source order.
func (e *DiffEntry) AppendHunk(h Hunk) {
	cp := h
	e.hunks = append(e.hunks, &cp)
}

// Hunks returns a snapshot of the entry's hunks (pointer copies; do not
// mutate fields directly — use ApproveHunk / RejectHunk on the parent
// DiffView so locking is observed).
func (e *DiffEntry) Hunks() []Hunk {
	out := make([]Hunk, len(e.hunks))
	for i, h := range e.hunks {
		out[i] = *h
	}
	return out
}

// Annotations returns a snapshot of all annotations on the entry.
func (e *DiffEntry) Annotations() []Annotation {
	out := make([]Annotation, len(e.annotations))
	copy(out, e.annotations)
	return out
}

// DiffView is the view-model holding the live list of diff entries.
//
// Safe for concurrent use: tool execution writes new entries while the UI
// thread reads / approves them.
type DiffView struct {
	mu       sync.RWMutex
	entries  []*DiffEntry
	index    map[string]*DiffEntry
	mode     DiffMode
	activeID string
}

// NewDiffView returns an empty view in unified mode.
func NewDiffView() *DiffView {
	return &DiffView{
		index: make(map[string]*DiffEntry),
		mode:  DiffModeUnified,
	}
}

// SetMode toggles between unified and side-by-side rendering.
func (v *DiffView) SetMode(m DiffMode) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.mode = m
}

// Mode returns the current rendering mode.
func (v *DiffView) Mode() DiffMode {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.mode
}

// Push appends a new entry. The most-recently pushed entry becomes active.
// Pushing an entry with an existing ID replaces it (e.g. the agent rewrote
// the same file).
func (v *DiffView) Push(e *DiffEntry) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if existing, ok := v.index[e.ID]; ok {
		// Replace in place to preserve order.
		for i, ent := range v.entries {
			if ent == existing {
				v.entries[i] = e
				break
			}
		}
	} else {
		v.entries = append(v.entries, e)
	}
	v.index[e.ID] = e
	v.activeID = e.ID
}

// Entries returns a snapshot of all entries in chronological order.
func (v *DiffView) Entries() []*DiffEntry {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]*DiffEntry, len(v.entries))
	copy(out, v.entries)
	return out
}

// Active returns the currently selected entry, or nil if the view is empty.
func (v *DiffView) Active() *DiffEntry {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.index[v.activeID]
}

// Select makes entryID the active entry.
func (v *DiffView) Select(entryID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.index[entryID]; ok {
		v.activeID = entryID
	}
}

// ApproveHunk marks a hunk approved. The renderer should hide approved hunks
// from the pending-review queue but keep them in the entry for audit.
func (v *DiffView) ApproveHunk(entryID, hunkID string) {
	v.mutateHunk(entryID, hunkID, HunkApproved)
}

// RejectHunk marks a hunk rejected — the agent's edit is discarded for that
// hunk and the rejection is reported back to the agent.
func (v *DiffView) RejectHunk(entryID, hunkID string) {
	v.mutateHunk(entryID, hunkID, HunkRejected)
}

func (v *DiffView) mutateHunk(entryID, hunkID string, status HunkStatus) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, ok := v.index[entryID]
	if !ok {
		return
	}
	for _, h := range e.hunks {
		if h.ID == hunkID {
			h.Status = status
			return
		}
	}
}

// Annotate attaches a free-form note to a specific line in a hunk. The note
// is fed to the agent on its next turn.
func (v *DiffView) Annotate(entryID, hunkID string, lineIdx int, body string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	e, ok := v.index[entryID]
	if !ok {
		return
	}
	e.annotations = append(e.annotations, Annotation{
		HunkID:  hunkID,
		LineIdx: lineIdx,
		Body:    body,
		At:      time.Now(),
	})
}

// PendingHunks returns hunks awaiting user disposition across all entries,
// ordered by entry insertion.
func (v *DiffView) PendingHunks() []HunkRef {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var out []HunkRef
	for _, e := range v.entries {
		for _, h := range e.hunks {
			if h.Status == HunkPending {
				out = append(out, HunkRef{EntryID: e.ID, HunkID: h.ID, Path: e.Path})
			}
		}
	}
	return out
}

// HunkRef identifies a hunk in a particular entry.
type HunkRef struct {
	EntryID string
	HunkID  string
	Path    string
}

// AnnotationsForAgent returns all user annotations across entries sorted
// chronologically. The coding-agent context assembler calls this each turn
// and embeds the results.
func (v *DiffView) AnnotationsForAgent() []Annotation {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var out []Annotation
	for _, e := range v.entries {
		out = append(out, e.annotations...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].At.Before(out[j].At)
	})
	return out
}

// Clear empties the view.
func (v *DiffView) Clear() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.entries = nil
	v.index = make(map[string]*DiffEntry)
	v.activeID = ""
}

// ParseUnifiedDiff parses a single-file unified diff (no `diff --git` header
// required) into a DiffEntry suitable for Push. The id, path, tracked, and
// baseSHA arguments come from the caller because they are not encoded in the
// hunk text.
//
// This is a conservative parser: it understands `@@ -a,b +c,d @@` headers
// and ` `, `+`, `-` line prefixes. Other markers (`\ No newline at end of
// file`, etc.) are skipped silently.
func ParseUnifiedDiff(id, path string, tracked bool, baseSHA, unified string) *DiffEntry {
	e := NewDiffEntry(id, path, tracked, baseSHA)
	var current *Hunk
	hunkSeq := 0
	oldLine, newLine := 0, 0

	for _, raw := range strings.Split(unified, "\n") {
		switch {
		case strings.HasPrefix(raw, "@@"):
			if current != nil {
				e.AppendHunk(*current)
			}
			oldStart, newStart := parseHunkHeader(raw)
			oldLine, newLine = oldStart, newStart
			hunkSeq++
			current = &Hunk{
				ID:       hunkID(id, hunkSeq),
				OldStart: oldStart,
				NewStart: newStart,
			}
		case current == nil:
			continue
		case strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++"):
			current.Lines = append(current.Lines, DiffLine{
				Kind: LineAdded, NewLine: newLine, Text: raw[1:],
			})
			newLine++
		case strings.HasPrefix(raw, "-") && !strings.HasPrefix(raw, "---"):
			current.Lines = append(current.Lines, DiffLine{
				Kind: LineRemoved, OldLine: oldLine, Text: raw[1:],
			})
			oldLine++
		case strings.HasPrefix(raw, " "):
			current.Lines = append(current.Lines, DiffLine{
				Kind: LineContext, OldLine: oldLine, NewLine: newLine, Text: raw[1:],
			})
			oldLine++
			newLine++
		}
	}
	if current != nil {
		e.AppendHunk(*current)
	}
	return e
}

// parseHunkHeader extracts the starting old and new line numbers from a
// `@@ -a,b +c,d @@` header. Missing values default to 1.
func parseHunkHeader(header string) (oldStart, newStart int) {
	oldStart, newStart = 1, 1
	// Trim leading "@@ " and trailing " @@<context>".
	rest := strings.TrimPrefix(header, "@@")
	if i := strings.Index(rest, "@@"); i >= 0 {
		rest = rest[:i]
	}
	for _, tok := range strings.Fields(rest) {
		if len(tok) < 2 {
			continue
		}
		switch tok[0] {
		case '-':
			oldStart = parseLeadingInt(tok[1:])
		case '+':
			newStart = parseLeadingInt(tok[1:])
		}
	}
	return
}

func parseLeadingInt(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return 1
	}
	return n
}

func hunkID(entryID string, seq int) string {
	return entryID + "#h" + itoa(seq)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

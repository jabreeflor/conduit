package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jabreeflor/conduit/internal/memory"
)

// MemoryInspector is the in-TUI memory browser. It owns its own state (entry
// list, filter query, cursor, mode) and is intentionally free of any Bubble
// Tea dependency — the same model is exercised by tests via plain method
// calls and surfaced in the interactive TUI through a thin Update wrapper.
//
// Modes:
//   - inspectorList    — browse + filter the entry list
//   - inspectorDetail  — full content of one entry
//   - inspectorConfirm — single-entry delete confirmation prompt
//   - inspectorPrune   — bulk-prune-by-filter confirmation prompt
type MemoryInspector struct {
	entries []memory.Entry // unfiltered, sorted snapshot from the provider
	filter  string         // live substring filter
	cursor  int            // index into the *filtered* slice
	mode    inspectorMode
	editing bool   // when true, keystrokes append to the filter
	last    string // last status / action message shown at the footer
}

type inspectorMode int

const (
	inspectorList inspectorMode = iota
	inspectorDetail
	inspectorConfirm
	inspectorPrune
)

// NewMemoryInspector returns an empty inspector in list mode.
func NewMemoryInspector() *MemoryInspector {
	return &MemoryInspector{}
}

// SetEntries replaces the entry list; cursor is clamped to the new length.
// Entries are sorted by UpdatedAt descending (newest first), with pinned
// entries surfaced ahead of unpinned within the same window so they're easy
// to find.
func (mi *MemoryInspector) SetEntries(entries []memory.Entry) {
	cp := make([]memory.Entry, len(entries))
	copy(cp, entries)
	sort.SliceStable(cp, func(i, j int) bool {
		if cp[i].Pinned != cp[j].Pinned {
			return cp[i].Pinned
		}
		return cp[i].UpdatedAt.After(cp[j].UpdatedAt)
	})
	mi.entries = cp
	mi.clampCursor()
}

// Filter returns the live filter query.
func (mi *MemoryInspector) Filter() string { return mi.filter }

// SetFilter sets the live substring filter. Resets cursor to top so the user
// always sees the first match.
func (mi *MemoryInspector) SetFilter(q string) {
	mi.filter = q
	mi.cursor = 0
}

// Mode reports the current view mode (list / detail / confirm / prune).
func (mi *MemoryInspector) Mode() inspectorMode { return mi.mode }

// Editing reports whether the filter input has focus.
func (mi *MemoryInspector) Editing() bool { return mi.editing }

// Filtered returns the entries matching the live filter, in display order.
func (mi *MemoryInspector) Filtered() []memory.Entry {
	if mi.filter == "" {
		return mi.entries
	}
	q := strings.ToLower(mi.filter)
	out := make([]memory.Entry, 0, len(mi.entries))
	for _, e := range mi.entries {
		if matchesFilter(e, q) {
			out = append(out, e)
		}
	}
	return out
}

// Cursor returns the current cursor position within the filtered slice.
func (mi *MemoryInspector) Cursor() int { return mi.cursor }

// Selected returns the currently highlighted entry, or false if there is none.
func (mi *MemoryInspector) Selected() (memory.Entry, bool) {
	f := mi.Filtered()
	if mi.cursor < 0 || mi.cursor >= len(f) {
		return memory.Entry{}, false
	}
	return f[mi.cursor], true
}

// CursorUp / CursorDown / CursorHome / CursorEnd move the highlight within the
// filtered slice. They are no-ops at the boundaries.
func (mi *MemoryInspector) CursorUp() {
	if mi.cursor > 0 {
		mi.cursor--
	}
}

func (mi *MemoryInspector) CursorDown() {
	if mi.cursor < len(mi.Filtered())-1 {
		mi.cursor++
	}
}

func (mi *MemoryInspector) CursorHome() { mi.cursor = 0 }

func (mi *MemoryInspector) CursorEnd() {
	n := len(mi.Filtered())
	if n == 0 {
		mi.cursor = 0
		return
	}
	mi.cursor = n - 1
}

// OpenDetail shows full content of the highlighted entry. No-op when nothing
// is selected.
func (mi *MemoryInspector) OpenDetail() {
	if _, ok := mi.Selected(); !ok {
		return
	}
	mi.mode = inspectorDetail
}

// CloseDetail returns to the list view.
func (mi *MemoryInspector) CloseDetail() { mi.mode = inspectorList }

// PromptDelete enters single-entry delete confirmation. No-op when nothing
// is selected.
func (mi *MemoryInspector) PromptDelete() {
	if _, ok := mi.Selected(); !ok {
		return
	}
	mi.mode = inspectorConfirm
}

// PromptPrune enters bulk-prune confirmation for the current filter. The
// confirmation summarises how many non-pinned entries will be removed.
func (mi *MemoryInspector) PromptPrune() { mi.mode = inspectorPrune }

// CancelPrompt dismisses any open confirmation modal.
func (mi *MemoryInspector) CancelPrompt() { mi.mode = inspectorList }

// StartFilterEdit gives the filter input focus.
func (mi *MemoryInspector) StartFilterEdit() {
	mi.editing = true
	mi.mode = inspectorList
}

// StopFilterEdit blurs the filter input.
func (mi *MemoryInspector) StopFilterEdit() { mi.editing = false }

// AppendFilter adds a single rune to the filter input — wired to keystrokes
// when editing is true.
func (mi *MemoryInspector) AppendFilter(r rune) {
	mi.filter += string(r)
	mi.cursor = 0
}

// BackspaceFilter removes the last rune from the filter (no-op when empty).
func (mi *MemoryInspector) BackspaceFilter() {
	if mi.filter == "" {
		return
	}
	rs := []rune(mi.filter)
	mi.filter = string(rs[:len(rs)-1])
	mi.cursor = 0
}

// LastMessage returns the most recent action result for the footer.
func (mi *MemoryInspector) LastMessage() string { return mi.last }

// SetMessage exposes the footer message slot to callers (engine wrappers
// surface delete / prune / pin results here).
func (mi *MemoryInspector) SetMessage(msg string) { mi.last = msg }

// PruneCandidates returns the entries that PromptPrune would actually remove
// given the current filter. Pinned entries are excluded — used by the prune
// confirmation prompt and tested directly.
func (mi *MemoryInspector) PruneCandidates() []memory.Entry {
	f := mi.Filtered()
	out := make([]memory.Entry, 0, len(f))
	for _, e := range f {
		if !e.Pinned {
			out = append(out, e)
		}
	}
	return out
}

// Render returns the full string view for the current mode. Width-aware
// formatting is intentionally minimal — the inspector is a single-column
// view that flows naturally inside the existing context-panel viewport.
func (mi *MemoryInspector) Render() string {
	switch mi.mode {
	case inspectorDetail:
		return mi.renderDetail()
	case inspectorConfirm:
		return mi.renderConfirm()
	case inspectorPrune:
		return mi.renderPruneConfirm()
	default:
		return mi.renderList()
	}
}

func (mi *MemoryInspector) renderList() string {
	var sb strings.Builder
	sb.WriteString("── memory inspector ──────────\n")
	sb.WriteString(mi.renderFilterLine() + "\n\n")

	filtered := mi.Filtered()
	if len(filtered) == 0 {
		if len(mi.entries) == 0 {
			sb.WriteString(" no memory entries\n")
		} else {
			sb.WriteString(" no entries match filter\n")
		}
	} else {
		for i, e := range filtered {
			caret := "  "
			if i == mi.cursor {
				caret = "> "
			}
			pin := " "
			if e.Pinned {
				pin = "📌"
			}
			ts := e.UpdatedAt.Format("2006-01-02 15:04")
			sb.WriteString(fmt.Sprintf("%s%s %s [%s] %s\n", caret, pin, ts, e.Kind, truncate(e.Title, 48)))
		}
	}

	sb.WriteString("\n" + mi.renderHelp())
	if mi.last != "" {
		sb.WriteString("\n " + mi.last)
	}
	return sb.String()
}

func (mi *MemoryInspector) renderDetail() string {
	e, ok := mi.Selected()
	if !ok {
		return mi.renderList()
	}
	var sb strings.Builder
	sb.WriteString("── memory entry ──────────────\n\n")
	fmt.Fprintf(&sb, " id:      %s\n", e.ID)
	fmt.Fprintf(&sb, " kind:    %s\n", e.Kind)
	fmt.Fprintf(&sb, " title:   %s\n", e.Title)
	fmt.Fprintf(&sb, " tags:    %s\n", strings.Join(e.Tags, ", "))
	fmt.Fprintf(&sb, " created: %s\n", e.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, " updated: %s\n", e.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, " pinned:  %t\n", e.Pinned)
	sb.WriteString("\n──── body ────\n\n")
	sb.WriteString(strings.TrimSpace(e.Body))
	sb.WriteString("\n\n esc: back   p: pin/unpin   d: delete\n")
	return sb.String()
}

func (mi *MemoryInspector) renderConfirm() string {
	e, ok := mi.Selected()
	if !ok {
		return mi.renderList()
	}
	return fmt.Sprintf(
		"── confirm delete ────────────\n\n"+
			" Delete this entry?\n\n"+
			"   %s [%s]\n   %s\n\n"+
			" y: confirm   n/esc: cancel\n",
		e.Title, e.Kind, e.ID,
	)
}

func (mi *MemoryInspector) renderPruneConfirm() string {
	candidates := mi.PruneCandidates()
	label := "all non-pinned entries"
	if mi.filter != "" {
		label = fmt.Sprintf("entries matching %q (non-pinned)", mi.filter)
	}
	return fmt.Sprintf(
		"── confirm prune ─────────────\n\n"+
			" Delete %d %s?\n\n"+
			" Pinned entries are skipped.\n\n"+
			" y: confirm   n/esc: cancel\n",
		len(candidates), label,
	)
}

func (mi *MemoryInspector) renderFilterLine() string {
	cursor := ""
	if mi.editing {
		cursor = "▌"
	}
	if mi.filter == "" && !mi.editing {
		return " filter: (none)   /: edit"
	}
	return fmt.Sprintf(" filter: %s%s", mi.filter, cursor)
}

func (mi *MemoryInspector) renderHelp() string {
	if mi.editing {
		return " enter/esc: stop editing   backspace: remove"
	}
	return " ↑/↓: move  enter: open  /: filter  p: pin  d: delete  P: prune  esc: close"
}

func (mi *MemoryInspector) clampCursor() {
	n := len(mi.Filtered())
	if n == 0 {
		mi.cursor = 0
		return
	}
	if mi.cursor < 0 {
		mi.cursor = 0
	}
	if mi.cursor >= n {
		mi.cursor = n - 1
	}
}

// matchesFilter is the inspector's local search predicate. Mirrors the
// FlatFileProvider's substring rules so pruning by filter has no surprises.
func matchesFilter(e memory.Entry, qLower string) bool {
	if strings.Contains(strings.ToLower(e.Title), qLower) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Body), qLower) {
		return true
	}
	if strings.Contains(strings.ToLower(string(e.Kind)), qLower) {
		return true
	}
	for _, t := range e.Tags {
		if strings.Contains(strings.ToLower(t), qLower) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

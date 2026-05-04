package coding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// defaultCompactThreshold is the input-window utilization fraction at which
// the budget signals reactive compaction. 80% leaves headroom for one more
// turn's input + tool results before re-compacting; tuning this lower hurts
// throughput (more frequent compactions), tuning it higher risks blowing
// the window mid-turn.
const defaultCompactThreshold = 0.80

// defaultPasteThreshold is the size at which a paragraph is replaced with a
// chip. 500 chars is the documented PRD threshold; below it the user can
// reasonably read the content inline in the transcript.
const defaultPasteThreshold = 500

// Budget tracks token usage relative to the configured input window so the
// REPL knows when to fire compaction. It also exposes a reactive flag for
// the "prompt too long" signal returned by some providers — that path
// triggers compaction even when we are nominally below the threshold,
// because the provider's own token accounting is the authoritative one.
type Budget struct {
	mu               sync.Mutex
	modelInputWindow int
	threshold        float64
	usedInput        int
	usedOutput       int
	promptTooLong    atomic.Bool
}

// BudgetSnapshot is a read-only view of Budget state for status surfaces.
type BudgetSnapshot struct {
	ModelInputWindow int
	Threshold        float64
	UsedInput        int
	UsedOutput       int
	PromptTooLong    bool
	ShouldCompact    bool
}

// NewBudget returns a Budget with the default 80% compaction threshold.
func NewBudget(window int) *Budget {
	return &Budget{
		modelInputWindow: window,
		threshold:        defaultCompactThreshold,
	}
}

// WithThreshold overrides the compaction threshold. Exposed for tests and
// for advanced users who want to compact earlier on small models.
func (b *Budget) WithThreshold(f float64) *Budget {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.threshold = f
	return b
}

// Observe accumulates per-turn token counts. Counters are additive across
// the session until Reset is called after a successful compaction.
func (b *Budget) Observe(inputTokens, outputTokens int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.usedInput += inputTokens
	b.usedOutput += outputTokens
}

// MarkPromptTooLong sets the reactive flag. Use this when a provider
// rejects a turn for context-length reasons — it forces ShouldCompact to
// return true even when usage is below the threshold.
func (b *Budget) MarkPromptTooLong() {
	b.promptTooLong.Store(true)
}

// Compaction trigger: hybrid — fires at 80% of input window OR on a reactive prompt-too-long signal. 80% leaves headroom for one more turn's input + tool results before re-compacting.
func (b *Budget) ShouldCompact() bool {
	if b.promptTooLong.Load() {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.modelInputWindow <= 0 {
		return false
	}
	limit := int(float64(b.modelInputWindow) * b.threshold)
	return b.usedInput >= limit
}

// Reset zeroes counters and clears the reactive flag. Called after the
// compactor has finished rewriting the prompt so subsequent turns observe
// against the post-compaction baseline.
func (b *Budget) Reset() {
	b.mu.Lock()
	b.usedInput = 0
	b.usedOutput = 0
	b.mu.Unlock()
	b.promptTooLong.Store(false)
}

// Snapshot returns a consistent read of the budget state.
func (b *Budget) Snapshot() BudgetSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	limit := 0
	if b.modelInputWindow > 0 {
		limit = int(float64(b.modelInputWindow) * b.threshold)
	}
	shouldCompact := b.promptTooLong.Load() || (b.modelInputWindow > 0 && b.usedInput >= limit)
	return BudgetSnapshot{
		ModelInputWindow: b.modelInputWindow,
		Threshold:        b.threshold,
		UsedInput:        b.usedInput,
		UsedOutput:       b.usedOutput,
		PromptTooLong:    b.promptTooLong.Load(),
		ShouldCompact:    shouldCompact,
	}
}

// PasteChip is the metadata stored for each collapsed paste. Text holds the
// original content so ExpandChips can reverse the substitution; the other
// fields are for display surfaces that want a lightweight summary.
type PasteChip struct {
	ID            string
	Length        int
	Sha256Prefix8 string
	FirstLine     string
	Text          string
}

// CollapsePastes replaces blocks of >= threshold characters (split on blank
// lines) with `[paste:chipID]` placeholders. If threshold is <= 0 the
// default of 500 is used.
//
// The implementation is intentionally simple: no language-aware parsing,
// no nested paste sentinels, no overlap handling. Tests pin the exact
// behavior; callers that need richer detection should layer a parser on
// top before invoking this function.
func CollapsePastes(s string, threshold int) (string, []PasteChip) {
	if threshold <= 0 {
		threshold = defaultPasteThreshold
	}

	// Split on blank-line boundaries while preserving them in the output.
	blocks := splitOnBlankLines(s)
	var chips []PasteChip
	var out strings.Builder
	out.Grow(len(s))

	for _, block := range blocks {
		if len(block.text) >= threshold {
			chip := newPasteChip(len(chips), block.text)
			chips = append(chips, chip)
			out.WriteString(block.leading)
			out.WriteString("[paste:" + chip.ID + "]")
			out.WriteString(block.trailing)
			continue
		}
		out.WriteString(block.leading)
		out.WriteString(block.text)
		out.WriteString(block.trailing)
	}
	return out.String(), chips
}

type pasteBlock struct {
	leading  string // blank-line whitespace preceding the block
	text     string // the block content (no surrounding blank lines)
	trailing string // blank-line whitespace following the block
}

// splitOnBlankLines walks runes and yields blocks separated by runs of
// blank lines (>= 2 consecutive newlines). Leading/trailing whitespace per
// block is captured so the reassembled output is byte-identical when no
// chips are produced.
func splitOnBlankLines(s string) []pasteBlock {
	if s == "" {
		return nil
	}
	var blocks []pasteBlock
	i := 0
	for i < len(s) {
		// Capture whitespace separator (anything that contains blank lines).
		start := i
		for i < len(s) {
			r := s[i]
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				i++
				continue
			}
			break
		}
		leading := s[start:i]

		// Capture body text up to next blank line (\n\n or \r\n\r\n).
		bodyStart := i
		for i < len(s) {
			if i+1 < len(s) && s[i] == '\n' && s[i+1] == '\n' {
				break
			}
			if i+3 < len(s) && s[i] == '\r' && s[i+1] == '\n' && s[i+2] == '\r' && s[i+3] == '\n' {
				break
			}
			i++
		}
		text := s[bodyStart:i]
		if leading == "" && text == "" {
			break
		}
		blocks = append(blocks, pasteBlock{leading: leading, text: text})
	}
	return blocks
}

func newPasteChip(index int, text string) PasteChip {
	sum := sha256.Sum256([]byte(text))
	prefix := hex.EncodeToString(sum[:4]) // 8 hex chars
	first := text
	if nl := strings.IndexByte(text, '\n'); nl >= 0 {
		first = text[:nl]
	}
	first = strings.TrimSpace(first)
	if len(first) > 80 {
		first = first[:80]
	}
	return PasteChip{
		ID:            fmt.Sprintf("p%d-%s", index, prefix),
		Length:        len(text),
		Sha256Prefix8: prefix,
		FirstLine:     first,
		Text:          text,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Tokenizer-aware accounting
// ──────────────────────────────────────────────────────────────────────────────

// EstimateTokens returns an approximate BPE token count for text.
// The heuristic (len+3)/4 rounds up to the nearest whole token and matches the
// standard GPT-family approximation used throughout the PRD budget tables.
func EstimateTokens(text string) int {
	return (len(text) + 3) / 4
}

// ──────────────────────────────────────────────────────────────────────────────
// PasteChip re-expansion
// ──────────────────────────────────────────────────────────────────────────────

// ExpandChips reverses CollapsePastes: every [paste:chipID] placeholder in s
// is replaced with the original text stored in the matching chip.
func ExpandChips(s string, chips []PasteChip) string {
	for _, chip := range chips {
		s = strings.ReplaceAll(s, "[paste:"+chip.ID+"]", chip.Text)
	}
	return s
}

// ──────────────────────────────────────────────────────────────────────────────
// CLAUDE.md / .conduit/rules/*.md discovery
// ──────────────────────────────────────────────────────────────────────────────

// ContextFile pairs an on-disk path with the file's contents.
type ContextFile struct {
	Path    string
	Content string
}

// DiscoverContextFiles walks the directory tree from projectRoot to the
// filesystem root collecting CLAUDE.md files (project-local first), then reads
// all *.md files under .conduit/rules/ in both projectRoot and homeDir.
//
// Missing files and directories are silently skipped; only genuine I/O errors
// are returned so callers can operate in repos that have no context files at
// all.
func DiscoverContextFiles(projectRoot, homeDir string) ([]ContextFile, error) {
	var result []ContextFile

	dir := filepath.Clean(projectRoot)
	for {
		claudePath := filepath.Join(dir, "CLAUDE.md")
		if data, err := os.ReadFile(claudePath); err == nil {
			result = append(result, ContextFile{Path: claudePath, Content: string(data)})
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	for _, root := range []string{projectRoot, homeDir} {
		rulesDir := filepath.Join(root, ".conduit", "rules")
		entries, err := os.ReadDir(rulesDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(rulesDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			result = append(result, ContextFile{Path: path, Content: string(data)})
		}
	}

	return result, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Auto-snip
// ──────────────────────────────────────────────────────────────────────────────

// Snip trims the oldest non-system turns from the conversation so the estimated
// token total fits within the budget threshold. turns[0] is always preserved as
// the system-context anchor. maxKeepTurns = 0 disables the hard count cap.
func (b *Budget) Snip(turns []contracts.CodingTurn, maxKeepTurns int) []contracts.CodingTurn {
	if len(turns) <= 1 {
		return turns
	}

	result := make([]contracts.CodingTurn, len(turns))
	copy(result, turns)

	if maxKeepTurns > 0 && len(result) > maxKeepTurns {
		keep := maxKeepTurns - 1
		tail := result[len(result)-keep:]
		trimmed := make([]contracts.CodingTurn, 1+len(tail))
		trimmed[0] = result[0]
		copy(trimmed[1:], tail)
		result = trimmed
	}

	b.mu.Lock()
	window := b.modelInputWindow
	thresh := b.threshold
	b.mu.Unlock()

	if window <= 0 {
		return result
	}

	limit := int(float64(window) * thresh)
	total := 0
	for _, t := range result {
		total += EstimateTokens(t.Content)
	}

	for total > limit && len(result) > 1 {
		total -= EstimateTokens(result[1].Content)
		result = append(result[:1:1], result[2:]...)
	}

	return result
}

// ──────────────────────────────────────────────────────────────────────────────
// Compactor interface + RunCompaction
// ──────────────────────────────────────────────────────────────────────────────

// Compactor condenses a conversation history into a summary string that replaces
// the full turn list after compaction.
type Compactor interface {
	Compact(ctx context.Context, turns []contracts.CodingTurn) (string, error)
}

// DefaultCompactor is the no-op fallback used when no summarisation model is
// configured; it acknowledges that compaction occurred without producing a real
// summary.
type DefaultCompactor struct{}

// Compact implements Compactor.
func (DefaultCompactor) Compact(_ context.Context, _ []contracts.CodingTurn) (string, error) {
	return "Context compacted.", nil
}

// RunCompaction calls c.Compact to summarise turns, resets the budget, then
// returns a single-element slice holding the summary as an assistant turn.
func (b *Budget) RunCompaction(ctx context.Context, turns []contracts.CodingTurn, c Compactor) ([]contracts.CodingTurn, error) {
	summary, err := c.Compact(ctx, turns)
	if err != nil {
		return nil, err
	}
	b.Reset()
	return []contracts.CodingTurn{{Role: "assistant", Content: summary}}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Preflight
// ──────────────────────────────────────────────────────────────────────────────

// PreflightResult reports the estimated cost of a prompt against the current
// budget state before the turn is actually sent.
type PreflightResult struct {
	EstimatedTokens int
	ShouldCompact   bool
	OverBudget      bool
	BudgetSnap      BudgetSnapshot
}

// Preflight estimates the token cost of prompt and checks it against the
// current budget. OverBudget is true when the prompt would push cumulative
// input past the model's absolute input window — the compaction threshold is a
// softer signal captured by ShouldCompact.
func Preflight(budget *Budget, prompt string) PreflightResult {
	snap := budget.Snapshot()
	est := EstimateTokens(prompt)
	overBudget := snap.ModelInputWindow > 0 && (snap.UsedInput+est) > snap.ModelInputWindow
	return PreflightResult{
		EstimatedTokens: est,
		ShouldCompact:   snap.ShouldCompact,
		OverBudget:      overBudget,
		BudgetSnap:      snap,
	}
}

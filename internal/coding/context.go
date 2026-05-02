package coding

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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

// PasteChip is the metadata stored for each collapsed paste. The full text
// stays accessible via the chip ID so a follow-up tool call can re-expand
// it; for now only the prefix and first line are retained for surfaces.
type PasteChip struct {
	ID            string
	Length        int
	Sha256Prefix8 string
	FirstLine     string
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
	}
}

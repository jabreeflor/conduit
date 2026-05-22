// Package contextassembler optimizes raw session context before model calls.
package contextassembler

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Category identifies the source and budget lane for a context item.
type Category string

const (
	CategorySystem       Category = "system"
	CategoryRecent       Category = "recent_conversation"
	CategoryHistory      Category = "history"
	CategoryMemory       Category = "memory"
	CategoryFile         Category = "file"
	CategoryDiff         Category = "diff"
	CategoryTool         Category = "tool"
	CategoryMultimodal   Category = "multimodal"
	CategoryInstructions Category = "instructions"
)

// DefaultCategoryBudgets allocates a modest prompt budget by context class.
var DefaultCategoryBudgets = map[Category]int{
	CategorySystem:       1_500,
	CategoryInstructions: 1_500,
	CategoryRecent:       4_000,
	CategoryHistory:      2_000,
	CategoryMemory:       2_000,
	CategoryFile:         6_000,
	CategoryDiff:         3_000,
	CategoryTool:         2_000,
	CategoryMultimodal:   1_000,
}

// Item is one raw context fragment considered for prompt assembly.
type Item struct {
	ID       string
	Category Category
	Content  string
	Path     string
	Pinned   bool
	Metadata map[string]string
}

// Request is the raw context presented to the assembler.
type Request struct {
	Query           string
	Items           []Item
	CategoryBudgets map[Category]int
	MaxTokens       int
	RecentItems     int
}

// Decision records what happened to one context item.
type Decision struct {
	ID             string
	Category       Category
	OriginalTokens int
	FinalTokens    int
	Score          float64
	Included       bool
	Action         string
}

// Summary is a transparent optimization report for session logs and tests.
type Summary struct {
	OriginalTokens  int
	FinalTokens     int
	DroppedItems    int
	SummarizedItems int
	DiffedItems     int
	ExtractedItems  int
	Decisions       []Decision
}

// Result is the optimized prompt and its accounting details.
type Result struct {
	Prompt  string
	Items   []Item
	Summary Summary
}

// TokenEstimator estimates prompt tokens without provider-specific APIs.
type TokenEstimator interface {
	EstimateTokens(string) int
}

// Summarizer compresses stale context before low-relevance items are dropped.
type Summarizer interface {
	Summarize(Item, int) Item
}

// CodeExtractor narrows source files to relevant declarations or blocks.
//
// Implementations may use Tree-sitter. The default extractor uses Go's AST for
// Go files and a line-window fallback for other source files.
type CodeExtractor interface {
	Extract(Item, string, int) (Item, bool)
}

// Assembler owns the optimization pipeline that runs before inference.
type Assembler struct {
	estimator  TokenEstimator
	summarizer Summarizer
	extractor  CodeExtractor
}

// New creates an assembler with deterministic local optimizers.
func New() *Assembler {
	return &Assembler{
		estimator:  CharEstimator{},
		summarizer: BoundarySummarizer{},
		extractor:  ASTExtractor{},
	}
}

// Assemble scores, compresses, budgets, and renders context into a prompt.
func (a *Assembler) Assemble(req Request) Result {
	if a == nil {
		a = New()
	}
	if a.estimator == nil {
		a.estimator = CharEstimator{}
	}
	if a.summarizer == nil {
		a.summarizer = BoundarySummarizer{}
	}
	if a.extractor == nil {
		a.extractor = ASTExtractor{}
	}

	budgets := mergeBudgets(req.CategoryBudgets)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		for _, n := range budgets {
			maxTokens += n
		}
	}
	recentItems := req.RecentItems
	if recentItems <= 0 {
		recentItems = 8
	}

	candidates := make([]scoredItem, 0, len(req.Items))
	var summary Summary
	for i, raw := range req.Items {
		item := normalizeItem(raw)
		origTokens := a.estimator.EstimateTokens(item.Content)
		summary.OriginalTokens += origTokens
		score := relevanceScore(req.Query, item, i, len(req.Items), recentItems)
		action := "kept"

		if prev := item.Metadata["previous_content"]; prev != "" && item.Content != "" {
			if diff := lineDiff(prev, item.Content); diff != "" {
				item.Content = diff
				item.Category = CategoryDiff
				action = "diffed"
				summary.DiffedItems++
			}
		}

		if item.Category == CategoryFile {
			budget := budgets[CategoryFile]
			if extracted, ok := a.extractor.Extract(item, req.Query, budget); ok {
				item = extracted
				action = "ast-extracted"
				summary.ExtractedItems++
			}
		}

		tokens := a.estimator.EstimateTokens(item.Content)
		if item.Category == CategoryHistory && tokens > budgets[CategoryHistory]/2 {
			item = a.summarizer.Summarize(item, budgets[CategoryHistory]/2)
			action = "summarized"
			summary.SummarizedItems++
			tokens = a.estimator.EstimateTokens(item.Content)
		}

		candidates = append(candidates, scoredItem{
			Item:           item,
			OriginalTokens: origTokens,
			FinalTokens:    tokens,
			Score:          score,
			Action:         action,
			OriginalIndex:  i,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].OriginalIndex < candidates[j].OriginalIndex
		}
		return candidates[i].Score > candidates[j].Score
	})

	usedByCategory := map[Category]int{}
	usedTotal := 0
	selected := make([]scoredItem, 0, len(candidates))
	for _, candidate := range candidates {
		catBudget := budgets[candidate.Category]
		if catBudget <= 0 {
			catBudget = maxTokens
		}
		fitsCategory := usedByCategory[candidate.Category]+candidate.FinalTokens <= catBudget
		fitsTotal := usedTotal+candidate.FinalTokens <= maxTokens
		if candidate.Pinned || (fitsCategory && fitsTotal) {
			selected = append(selected, candidate)
			usedByCategory[candidate.Category] += candidate.FinalTokens
			usedTotal += candidate.FinalTokens
			summary.Decisions = append(summary.Decisions, candidate.decision(true))
			continue
		}
		summary.DroppedItems++
		dropped := candidate.decision(false)
		dropped.Action = "dropped"
		summary.Decisions = append(summary.Decisions, dropped)
	}

	sort.SliceStable(selected, func(i, j int) bool {
		return selected[i].OriginalIndex < selected[j].OriginalIndex
	})

	items := make([]Item, 0, len(selected))
	parts := make([]string, 0, len(selected))
	for _, selectedItem := range selected {
		items = append(items, selectedItem.Item)
		parts = append(parts, renderItem(selectedItem.Item))
	}
	prompt := strings.TrimSpace(strings.Join(parts, "\n\n"))
	summary.FinalTokens = a.estimator.EstimateTokens(prompt)
	return Result{Prompt: prompt, Items: items, Summary: summary}
}

type scoredItem struct {
	Item
	OriginalTokens int
	FinalTokens    int
	Score          float64
	Action         string
	OriginalIndex  int
}

func (s scoredItem) decision(included bool) Decision {
	return Decision{
		ID:             s.ID,
		Category:       s.Category,
		OriginalTokens: s.OriginalTokens,
		FinalTokens:    s.FinalTokens,
		Score:          s.Score,
		Included:       included,
		Action:         s.Action,
	}
}

// CharEstimator approximates common BPE tokenization at four bytes per token.
type CharEstimator struct{}

func (CharEstimator) EstimateTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return int(math.Ceil(float64(len(s)) / 4.0))
}

// BoundarySummarizer keeps the head and tail of older context.
type BoundarySummarizer struct{}

func (BoundarySummarizer) Summarize(item Item, targetTokens int) Item {
	if targetTokens <= 0 {
		targetTokens = 250
	}
	targetChars := targetTokens * 4
	text := strings.TrimSpace(item.Content)
	if len(text) <= targetChars {
		return item
	}
	half := targetChars / 2
	item.Content = strings.TrimSpace(text[:half]) + "\n[...summarized older context...]\n" + strings.TrimSpace(text[len(text)-half:])
	return item
}

// ASTExtractor extracts relevant Go declarations, falling back to line windows.
type ASTExtractor struct{}

func (ASTExtractor) Extract(item Item, query string, tokenBudget int) (Item, bool) {
	if tokenBudget <= 0 || item.Content == "" {
		return item, false
	}
	terms := queryTerms(query + " " + filepath.Base(item.Path))
	if len(terms) == 0 {
		return item, false
	}
	if strings.HasSuffix(item.Path, ".go") {
		if extracted, ok := extractGoDeclarations(item, terms, tokenBudget); ok {
			return extracted, true
		}
	}
	if extracted, ok := extractLineWindows(item, terms, tokenBudget); ok {
		return extracted, true
	}
	return item, false
}

func extractGoDeclarations(item Item, terms map[string]struct{}, tokenBudget int) (Item, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, item.Path, item.Content, parser.ParseComments)
	if err != nil {
		return item, false
	}
	type match struct {
		start int
		end   int
		name  string
	}
	var matches []match
	for _, decl := range file.Decls {
		name := declarationName(decl)
		if name == "" || !containsAnyTerm(name, terms) {
			continue
		}
		start := fset.Position(decl.Pos()).Offset
		end := fset.Position(decl.End()).Offset
		if start >= 0 && end > start && end <= len(item.Content) {
			matches = append(matches, match{start: start, end: end, name: name})
		}
	}
	if len(matches) == 0 {
		return item, false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// AST excerpts from %s\n", item.Path)
	for _, m := range matches {
		if (CharEstimator{}).EstimateTokens(b.String()+item.Content[m.start:m.end]) > tokenBudget {
			break
		}
		fmt.Fprintf(&b, "\n// %s\n%s\n", m.name, strings.TrimSpace(item.Content[m.start:m.end]))
	}
	if strings.TrimSpace(b.String()) == "" {
		return item, false
	}
	item.Content = strings.TrimSpace(b.String())
	return item, true
}

func declarationName(decl ast.Decl) string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv != nil && len(d.Recv.List) > 0 {
			return receiverName(d.Recv.List[0]) + "." + d.Name.Name
		}
		return d.Name.Name
	case *ast.GenDecl:
		var names []string
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, s.Name.Name)
			case *ast.ValueSpec:
				for _, name := range s.Names {
					names = append(names, name.Name)
				}
			}
		}
		return strings.Join(names, " ")
	default:
		return ""
	}
}

func receiverName(field *ast.Field) string {
	if field == nil {
		return ""
	}
	switch t := field.Type.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func extractLineWindows(item Item, terms map[string]struct{}, tokenBudget int) (Item, bool) {
	lines := strings.Split(item.Content, "\n")
	var b strings.Builder
	for i, line := range lines {
		if !containsAnyTerm(line, terms) {
			continue
		}
		start := max(0, i-3)
		end := min(len(lines), i+4)
		window := fmt.Sprintf("// excerpt %s:%d\n%s\n", item.Path, start+1, strings.Join(lines[start:end], "\n"))
		if (CharEstimator{}).EstimateTokens(b.String()+window) > tokenBudget {
			break
		}
		b.WriteString(window)
	}
	if strings.TrimSpace(b.String()) == "" {
		return item, false
	}
	item.Content = strings.TrimSpace(b.String())
	return item, true
}

func relevanceScore(query string, item Item, index, total, recentItems int) float64 {
	score := categoryWeight(item.Category)
	if item.Pinned {
		score += 1_000
	}
	if total-index <= recentItems && (item.Category == CategoryRecent || item.Category == CategoryHistory) {
		score += 30
	}
	terms := queryTerms(query)
	searchable := strings.ToLower(item.ID + " " + item.Path + " " + item.Content)
	for term := range terms {
		score += float64(strings.Count(searchable, term)) * 3
	}
	return score
}

func categoryWeight(category Category) float64 {
	switch category {
	case CategorySystem, CategoryInstructions:
		return 100
	case CategoryRecent:
		return 80
	case CategoryDiff:
		return 75
	case CategoryFile:
		return 65
	case CategoryMemory:
		return 45
	case CategoryTool:
		return 35
	case CategoryMultimodal:
		return 30
	case CategoryHistory:
		return 20
	default:
		return 10
	}
}

func queryTerms(s string) map[string]struct{} {
	terms := map[string]struct{}{}
	for _, field := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	}) {
		if len(field) < 3 {
			continue
		}
		terms[field] = struct{}{}
	}
	return terms
}

func containsAnyTerm(s string, terms map[string]struct{}) bool {
	lower := strings.ToLower(s)
	for term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func mergeBudgets(overrides map[Category]int) map[Category]int {
	budgets := make(map[Category]int, len(DefaultCategoryBudgets)+len(overrides))
	for category, budget := range DefaultCategoryBudgets {
		budgets[category] = budget
	}
	for category, budget := range overrides {
		if budget > 0 {
			budgets[category] = budget
		}
	}
	return budgets
}

func normalizeItem(item Item) Item {
	if item.Category == "" {
		item.Category = CategoryHistory
	}
	if item.Metadata == nil {
		item.Metadata = map[string]string{}
	}
	if item.ID == "" {
		item.ID = item.Path
	}
	if item.ID == "" {
		item.ID = string(item.Category)
	}
	return item
}

func renderItem(item Item) string {
	label := string(item.Category)
	if item.Path != "" {
		label += ": " + item.Path
	} else if item.ID != "" {
		label += ": " + item.ID
	}
	return fmt.Sprintf("<%s>\n%s\n</%s>", label, strings.TrimSpace(item.Content), item.Category)
}

func lineDiff(previous, current string) string {
	prev := strings.Split(previous, "\n")
	next := strings.Split(current, "\n")
	maxLines := max(len(prev), len(next))
	var b strings.Builder
	for i := 0; i < maxLines; i++ {
		var oldLine, newLine string
		if i < len(prev) {
			oldLine = prev[i]
		}
		if i < len(next) {
			newLine = next[i]
		}
		if oldLine == newLine {
			continue
		}
		if oldLine != "" {
			fmt.Fprintf(&b, "-%s\n", oldLine)
		}
		if newLine != "" {
			fmt.Fprintf(&b, "+%s\n", newLine)
		}
	}
	return strings.TrimSpace(b.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

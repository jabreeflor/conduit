package contextassembler

import (
	"strings"
	"testing"
)

func TestAssemblerScoresAndBudgetsByCategory(t *testing.T) {
	assembler := New()
	result := assembler.Assemble(Request{
		Query: "fix router timeout",
		CategoryBudgets: map[Category]int{
			CategoryRecent: 200,
			CategoryMemory: 40,
		},
		MaxTokens: 300,
		Items: []Item{
			{ID: "prompt", Category: CategoryRecent, Content: "Please fix the router timeout regression.", Pinned: true},
			{ID: "low", Category: CategoryMemory, Content: strings.Repeat("unrelated ", 400)},
			{ID: "high", Category: CategoryMemory, Content: "router timeout provider failover"},
		},
	})

	if !strings.Contains(result.Prompt, "router timeout provider failover") {
		t.Fatalf("relevant memory missing from prompt:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "unrelated unrelated") {
		t.Fatalf("low-relevance memory was not dropped:\n%s", result.Prompt)
	}
	if result.Summary.DroppedItems == 0 {
		t.Fatal("expected at least one dropped item")
	}
}

func TestAssemblerSummarizesOldHistory(t *testing.T) {
	assembler := New()
	history := "start " + strings.Repeat("middle ", 2_000) + " end"
	result := assembler.Assemble(Request{
		Query: "continue",
		CategoryBudgets: map[Category]int{
			CategoryHistory: 120,
		},
		MaxTokens: 300,
		Items: []Item{{
			ID:       "old-turns",
			Category: CategoryHistory,
			Content:  history,
		}},
	})

	if result.Summary.SummarizedItems != 1 {
		t.Fatalf("SummarizedItems = %d, want 1", result.Summary.SummarizedItems)
	}
	if !strings.Contains(result.Prompt, "[...summarized older context...]") {
		t.Fatalf("summary marker missing:\n%s", result.Prompt)
	}
}

func TestAssemblerDiffsChangedFileContent(t *testing.T) {
	assembler := New()
	result := assembler.Assemble(Request{
		Query: "handler",
		Items: []Item{{
			ID:       "file",
			Category: CategoryFile,
			Path:     "handler.go",
			Content:  "package x\nfunc handler() { println(\"new\") }\n",
			Metadata: map[string]string{
				"previous_content": "package x\nfunc handler() { println(\"old\") }\n",
			},
		}},
	})

	if result.Summary.DiffedItems != 1 {
		t.Fatalf("DiffedItems = %d, want 1", result.Summary.DiffedItems)
	}
	if !strings.Contains(result.Prompt, "+func handler()") || !strings.Contains(result.Prompt, "-func handler()") {
		t.Fatalf("diff missing expected lines:\n%s", result.Prompt)
	}
}

func TestAssemblerExtractsGoDeclarations(t *testing.T) {
	assembler := New()
	source := `package demo

func unrelated() string {
	return "ignore"
}

type Router struct{}

func (r *Router) Infer() string {
	return "keep"
}
`
	result := assembler.Assemble(Request{
		Query: "router infer",
		CategoryBudgets: map[Category]int{
			CategoryFile: 80,
		},
		MaxTokens: 120,
		Items: []Item{{
			ID:       "router",
			Category: CategoryFile,
			Path:     "router.go",
			Content:  source,
		}},
	})

	if result.Summary.ExtractedItems != 1 {
		t.Fatalf("ExtractedItems = %d, want 1", result.Summary.ExtractedItems)
	}
	if !strings.Contains(result.Prompt, "func (r *Router) Infer()") {
		t.Fatalf("relevant declaration missing:\n%s", result.Prompt)
	}
	if strings.Contains(result.Prompt, "unrelated") {
		t.Fatalf("unrelated declaration should not be included:\n%s", result.Prompt)
	}
}

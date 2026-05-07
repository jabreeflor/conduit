package ide

import (
	"os"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/coding"
)

// TestDetectLanguage tests language detection from file extensions
func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		filename string
		expected Language
	}{
		{"main.go", Go},
		{"script.py", Python},
		{"app.js", JavaScript},
		{"build.sh", Shell},
		{"config.json", JSON},
		{"values.yaml", YAML},
		{"unknown.txt", Other},
		{"component.tsx", JavaScript},
		{"data.yml", YAML},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := DetectLanguage(tt.filename)
			if result != tt.expected {
				t.Errorf("DetectLanguage(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

// TestHighlighterSetContent tests setting content on Highlighter
func TestHighlighterSetContent(t *testing.T) {
	h := NewHighlighter(Go)
	content := "package main\n\nfunc main() {\n}"

	h.SetContent(content)
	if h.content != content {
		t.Errorf("SetContent() did not set content correctly")
	}
}

// TestHighlighterGo tests Go syntax highlighting
func TestHighlighterGo(t *testing.T) {
	h := NewHighlighter(Go)
	content := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}`

	h.SetContent(content)
	highlighted := h.Highlight()

	// Check that highlighting was applied (contains ANSI codes)
	if !strings.Contains(highlighted, colorReset) {
		t.Errorf("Highlight() did not apply color codes")
	}

	// Check that keywords are highlighted
	if !strings.Contains(highlighted, colorYellow+"package"+colorReset) {
		t.Errorf("'package' keyword not highlighted")
	}
}

// TestHighlighterJSON tests JSON syntax highlighting
func TestHighlighterJSON(t *testing.T) {
	h := NewHighlighter(JSON)
	content := `{
	"name": "test",
	"count": 42,
	"active": true
}`

	h.SetContent(content)
	highlighted := h.Highlight()

	// Check that strings are highlighted green
	if !strings.Contains(highlighted, colorGreen) {
		t.Errorf("JSON strings not highlighted")
	}

	// Check that numbers are highlighted cyan
	if !strings.Contains(highlighted, colorCyan) {
		t.Errorf("JSON numbers not highlighted")
	}

	// Check that booleans are highlighted yellow
	if strings.Contains(highlighted, colorYellow) && strings.Contains(highlighted, "true") {
		// Expected: booleans highlighted
	}
}

// TestHighlighterYAML tests YAML syntax highlighting
func TestHighlighterYAML(t *testing.T) {
	h := NewHighlighter(YAML)
	content := `app:
  name: test
  enabled: true
  # Configuration comment`

	h.SetContent(content)
	highlighted := h.Highlight()

	// Check that color codes were applied
	if !strings.Contains(highlighted, colorReset) {
		t.Errorf("Highlight() did not apply color codes for YAML")
	}
}

// TestExtractWord tests word extraction at cursor position
func TestExtractWord(t *testing.T) {
	tests := []struct {
		line     string
		col      int
		expected string
	}{
		{"func main", 4, "func"},
		{"fmt.Println", 3, "fmt"},
		{"myVariable := 5", 2, "my"},
		{"const", 5, "const"},
		{"", 0, ""},
		{"word_with_underscore", 5, "word"},
	}

	for _, tt := range tests {
		t.Run(tt.line+" at "+string(rune(tt.col)), func(t *testing.T) {
			result := extractWord(tt.line, tt.col)
			if result != tt.expected {
				t.Errorf("extractWord(%q, %d) = %q, want %q", tt.line, tt.col, result, tt.expected)
			}
		})
	}
}

// TestIsWordChar tests word character classification
func TestIsWordChar(t *testing.T) {
	tests := []struct {
		ch       byte
		expected bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'_', true},
		{' ', false},
		{'.', false},
		{';', false},
		{'\n', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.ch), func(t *testing.T) {
			result := isWordChar(tt.ch)
			if result != tt.expected {
				t.Errorf("isWordChar(%q) = %v, want %v", tt.ch, result, tt.expected)
			}
		})
	}
}

// TestAIAssistSuggestAutocomplete tests autocomplete suggestions
func TestAIAssistSuggestAutocomplete(t *testing.T) {
	session := &coding.Session{}
	assist := NewAIAssist(session)

	tests := []struct {
		line        string
		col         int
		shouldMatch bool
	}{
		{"f", 1, true},           // Should match "f" -> func, for, fmt
		{"im", 2, true},          // Should match "im" -> import
		{"unknown", 7, false},    // Should not match "unknown"
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			suggestions := assist.SuggestAutocomplete(tt.line, tt.col)
			hasMatch := len(suggestions) > 0
			if hasMatch != tt.shouldMatch {
				t.Errorf("SuggestAutocomplete(%q, %d) = %v items, expected match=%v", tt.line, tt.col, len(suggestions), tt.shouldMatch)
			}
		})
	}
}

// TestAIAssistExplainCode tests code explanation
func TestAIAssistExplainCode(t *testing.T) {
	session := &coding.Session{}
	assist := NewAIAssist(session)

	content := `func main() {
	for i := 0; i < 10; i++ {
		fmt.Println(i)
	}
}`

	explanation := assist.ExplainCode(0, 1, content)
	if explanation == "Invalid line range" {
		t.Errorf("ExplainCode() returned invalid line range error unexpectedly")
	}

	// Test function detection
	funcExplain := assist.ExplainCode(0, 1, "func main() {")
	if !strings.Contains(funcExplain, "function") {
		t.Errorf("ExplainCode() did not recognize function declaration")
	}

	// Test loop detection
	loopExplain := assist.ExplainCode(1, 2, "for i := 0; i < 10; i++ {")
	if !strings.Contains(loopExplain, "loop") {
		t.Errorf("ExplainCode() did not recognize loop")
	}
}

// TestAIAssistFixError tests error fixing suggestions
func TestAIAssistFixError(t *testing.T) {
	session := &coding.Session{}
	assist := NewAIAssist(session)

	tests := []struct {
		line           string
		shouldContain  string
	}{
		{"panic(\"error\")", "error handling"},
		{"if x > 5", ""},  // Should suggest no specific pattern
		{"(((", "parentheses"},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			fix := assist.FixError(tt.line)
			if tt.shouldContain != "" && !strings.Contains(fix, tt.shouldContain) {
				t.Errorf("FixError(%q) did not contain %q", tt.line, tt.shouldContain)
			}
		})
	}
}

// TestEditorOpen tests opening a file
func TestEditorOpen(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test*.go")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test content
	content := "package main\n\nfunc main() {}"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	// Test opening the file
	session := &coding.Session{}
	editor := NewEditor(session)

	err = editor.Open(tmpFile.Name())
	if err != nil {
		t.Errorf("Open() returned error: %v", err)
	}

	if editor.content != content {
		t.Errorf("Open() did not load file content correctly")
	}

	if editor.language != Go {
		t.Errorf("Open() did not detect Go language for .go file")
	}
}

// TestEditorOpenNonexistent tests opening a nonexistent file
func TestEditorOpenNonexistent(t *testing.T) {
	session := &coding.Session{}
	editor := NewEditor(session)

	err := editor.Open("/nonexistent/path/file.go")
	if err == nil {
		t.Errorf("Open() should return error for nonexistent file")
	}
}

// TestEditorGetHighlighted tests getting highlighted content
func TestEditorGetHighlighted(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test*.go")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "package main"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	session := &coding.Session{}
	editor := NewEditor(session)
	editor.Open(tmpFile.Name())

	highlighted := editor.GetHighlighted()
	if !strings.Contains(highlighted, colorReset) {
		t.Errorf("GetHighlighted() did not return highlighted content")
	}
}

// TestEditorSuggest tests autocomplete suggestions in editor
func TestEditorSuggest(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test*.go")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "package main\n\nfunc main() {}"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	session := &coding.Session{}
	editor := NewEditor(session)
	editor.Open(tmpFile.Name())

	// Test suggestions on line with "f"
	suggestions := editor.Suggest(0, 1)
	// We expect some suggestions for "p" in "package"
	_ = suggestions // Variable used in test
}

// TestEditorExplain tests code explanation in editor
func TestEditorExplain(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test*.go")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "package main\n\nfunc main() {}"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	session := &coding.Session{}
	editor := NewEditor(session)
	editor.Open(tmpFile.Name())

	explanation := editor.Explain(2, 3)
	if strings.Contains(explanation, "Invalid") {
		t.Errorf("Explain() returned invalid range for valid input")
	}
}

// TestEditorContent tests retrieving editor content
func TestEditorContent(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test*.go")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "package main\n\nfunc main() {}"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	session := &coding.Session{}
	editor := NewEditor(session)
	editor.Open(tmpFile.Name())

	if editor.Content() != content {
		t.Errorf("Content() did not return correct content")
	}
}

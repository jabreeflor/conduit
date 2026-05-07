package ide

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jabreeflor/conduit/internal/coding"
)

// Language represents a programming language for syntax highlighting
type Language int

const (
	Go Language = iota
	Python
	JavaScript
	Shell
	JSON
	YAML
	Other
)

// ANSI color codes for terminal output
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
)

// DetectLanguage determines the language based on file extension
func DetectLanguage(filename string) Language {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".go":
		return Go
	case ".py":
		return Python
	case ".js", ".jsx", ".ts", ".tsx":
		return JavaScript
	case ".sh", ".bash":
		return Shell
	case ".json":
		return JSON
	case ".yaml", ".yml":
		return YAML
	default:
		return Other
	}
}

// Highlighter provides syntax highlighting for code
type Highlighter struct {
	content  string
	language Language
}

// NewHighlighter creates a new Highlighter instance
func NewHighlighter(language Language) *Highlighter {
	return &Highlighter{
		language: language,
	}
}

// SetContent sets the code content to be highlighted
func (h *Highlighter) SetContent(content string) {
	h.content = content
}

// Highlight returns the highlighted version of the content with ANSI codes
func (h *Highlighter) Highlight() string {
	switch h.language {
	case Go:
		return h.highlightGo()
	case Python:
		return h.highlightPython()
	case JavaScript:
		return h.highlightJavaScript()
	case Shell:
		return h.highlightShell()
	case JSON:
		return h.highlightJSON()
	case YAML:
		return h.highlightYAML()
	default:
		return h.content
	}
}

// goKeywords are Go language keywords
var goKeywords = []string{
	"package", "import", "func", "type", "struct", "interface", "const", "var",
	"if", "else", "for", "switch", "case", "default", "return", "break", "continue",
	"defer", "go", "select", "chan", "range", "map", "slice", "string", "int", "bool",
	"float64", "uint", "byte", "rune", "error", "nil", "true", "false",
}

// pythonKeywords are Python language keywords
var pythonKeywords = []string{
	"def", "class", "import", "from", "as", "if", "elif", "else", "for", "while",
	"break", "continue", "return", "try", "except", "finally", "raise", "with",
	"lambda", "yield", "pass", "assert", "del", "is", "in", "and", "or", "not",
	"True", "False", "None", "async", "await",
}

// jsKeywords are JavaScript language keywords
var jsKeywords = []string{
	"function", "const", "let", "var", "if", "else", "for", "while", "do", "switch",
	"case", "break", "continue", "return", "try", "catch", "finally", "throw",
	"async", "await", "new", "this", "class", "extends", "import", "export",
	"default", "from", "static", "get", "set", "true", "false", "null", "undefined",
}

// shellKeywords are shell language keywords
var shellKeywords = []string{
	"if", "then", "else", "elif", "fi", "for", "while", "do", "done", "case",
	"esac", "in", "function", "return", "exit", "export", "local", "readonly",
	"declare", "typeset", "alias", "unalias",
}

// highlightGo applies Go syntax highlighting
func (h *Highlighter) highlightGo() string {
	return h.highlightWithKeywords(goKeywords)
}

// highlightPython applies Python syntax highlighting
func (h *Highlighter) highlightPython() string {
	return h.highlightWithKeywords(pythonKeywords)
}

// highlightJavaScript applies JavaScript syntax highlighting
func (h *Highlighter) highlightJavaScript() string {
	return h.highlightWithKeywords(jsKeywords)
}

// highlightShell applies shell syntax highlighting
func (h *Highlighter) highlightShell() string {
	return h.highlightWithKeywords(shellKeywords)
}

// highlightJSON applies JSON syntax highlighting
func (h *Highlighter) highlightJSON() string {
	result := h.content

	// Highlight strings
	result = regexp.MustCompile(`"[^"]*"`).ReplaceAllString(result, colorGreen+"$0"+colorReset)

	// Highlight numbers
	result = regexp.MustCompile(`\b\d+\.?\d*\b`).ReplaceAllString(result, colorCyan+"$0"+colorReset)

	// Highlight booleans and null
	result = regexp.MustCompile(`\b(true|false|null)\b`).ReplaceAllString(result, colorYellow+"$0"+colorReset)

	return result
}

// highlightYAML applies YAML syntax highlighting
func (h *Highlighter) highlightYAML() string {
	result := h.content

	// Highlight keys (text before colons)
	result = regexp.MustCompile(`^(\s*)([a-zA-Z_][a-zA-Z0-9_-]*):`).ReplaceAllString(result, "$1"+colorBlue+"$2"+colorReset+":")

	// Highlight strings
	result = regexp.MustCompile(`"[^"]*"`).ReplaceAllString(result, colorGreen+"$0"+colorReset)
	result = regexp.MustCompile(`'[^']*'`).ReplaceAllString(result, colorGreen+"$0"+colorReset)

	// Highlight booleans
	result = regexp.MustCompile(`\b(true|false|yes|no)\b`).ReplaceAllString(result, colorYellow+"$0"+colorReset)

	// Highlight comments
	result = regexp.MustCompile(`#.*$`).ReplaceAllString(result, colorMagenta+"$0"+colorReset)

	return result
}

// highlightWithKeywords applies highlighting using language keywords
func (h *Highlighter) highlightWithKeywords(keywords []string) string {
	result := h.content

	// Highlight keywords
	for _, kw := range keywords {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(kw) + `\b`)
		result = pattern.ReplaceAllString(result, colorYellow+kw+colorReset)
	}

	// Highlight strings (single and double quoted)
	result = regexp.MustCompile(`"[^"]*"`).ReplaceAllString(result, colorGreen+"$0"+colorReset)
	result = regexp.MustCompile(`'[^']*'`).ReplaceAllString(result, colorGreen+"$0"+colorReset)

	// Highlight numbers
	result = regexp.MustCompile(`\b\d+\.?\d*\b`).ReplaceAllString(result, colorCyan+"$0"+colorReset)

	// Highlight comments
	result = regexp.MustCompile(`//.*$`).ReplaceAllString(result, colorMagenta+"$0"+colorReset)
	result = regexp.MustCompile(`#.*$`).ReplaceAllString(result, colorMagenta+"$0"+colorReset)

	return result
}

// AIAssist provides AI-powered code assistance
type AIAssist struct {
	session *coding.Session
}

// NewAIAssist creates a new AIAssist instance
func NewAIAssist(session *coding.Session) *AIAssist {
	return &AIAssist{
		session: session,
	}
}

// SuggestAutocomplete provides autocomplete suggestions based on context
func (a *AIAssist) SuggestAutocomplete(cursorLine string, cursorCol int) []string {
	if cursorCol > len(cursorLine) {
		cursorCol = len(cursorLine)
	}

	// Extract word at cursor
	word := extractWord(cursorLine, cursorCol)

	// Common completions for partial words
	completions := map[string][]string{
		"f":    {"func", "for", "fmt.Println"},
		"im":   {"import", "in"},
		"if":   {"if", "interface"},
		"st":   {"struct", "string"},
		"t":    {"type", "true"},
		"pa":   {"package", "panic"},
		"pr":   {"print", "printf", "println"},
		"de":   {"defer", "default"},
		"re":   {"return", "range"},
		"ca":   {"case", "catch", "const"},
		"ch":   {"chan", "change"},
		"err": {"error", "err"},
	}

	if suggestions, ok := completions[strings.ToLower(word)]; ok {
		return suggestions
	}

	return []string{}
}

// ExplainCode provides an explanation of code snippet
func (a *AIAssist) ExplainCode(startLine, endLine int, content string) string {
	lines := strings.Split(content, "\n")
	if startLine < 0 || endLine > len(lines) || startLine > endLine {
		return "Invalid line range"
	}

	snippet := strings.Join(lines[startLine:endLine], "\n")

	// Heuristic-based explanation
	if strings.Contains(snippet, "func") {
		return "This appears to be a function declaration. It defines a named function that can be called elsewhere in the code."
	}
	if strings.Contains(snippet, "for") {
		return "This appears to be a loop. It iterates over a sequence or range, executing the body multiple times."
	}
	if strings.Contains(snippet, "if") {
		return "This appears to be a conditional statement. It executes code based on whether a condition is true or false."
	}
	if strings.Contains(snippet, "type") && strings.Contains(snippet, "struct") {
		return "This appears to be a struct definition. It defines a composite data type with named fields."
	}
	if strings.Contains(snippet, "interface") {
		return "This appears to be an interface definition. It defines a contract that types can implement."
	}

	return "This code snippet contains general logic that may perform calculations, manipulations, or other operations."
}

// FixError suggests fixes for common error patterns
func (a *AIAssist) FixError(line string) string {
	// Common error patterns and fixes
	if strings.Contains(line, "panic") && !strings.Contains(line, "recover") {
		return "Consider using error handling instead of panic. Use return error types when possible."
	}
	if strings.HasPrefix(strings.TrimSpace(line), "var ") && strings.Contains(line, ":=") {
		return "Mixing var declaration with := is incorrect. Use either 'var name = value' or 'name := value'."
	}
	if !strings.Contains(line, "err") && strings.Contains(line, "return") {
		return "Function returns error but no error handling is present."
	}
	if strings.Count(line, "(") != strings.Count(line, ")") {
		return "Mismatched parentheses. Check that all opening parentheses have matching closing ones."
	}

	return "Unable to identify specific error pattern. Review the code for syntax or logic errors."
}

// extractWord extracts the word at or before the cursor position
func extractWord(line string, cursorCol int) string {
	if cursorCol > len(line) {
		cursorCol = len(line)
	}

	start := cursorCol
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}

	end := cursorCol
	for end < len(line) && isWordChar(line[end]) {
		end++
	}

	if start > end {
		start = end
	}

	if start >= len(line) || end > len(line) || start >= end {
		return ""
	}

	return line[start:end]
}

// isWordChar checks if a character is part of a word
func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

// Editor is a composite IDE editor combining highlighting and AI assistance
type Editor struct {
	filePath   string
	content    string
	language   Language
	highlighter *Highlighter
	assist     *AIAssist
}

// NewEditor creates a new Editor instance
func NewEditor(session *coding.Session) *Editor {
	return &Editor{
		assist: NewAIAssist(session),
	}
}

// Open loads and opens a file for editing
func (e *Editor) Open(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	e.filePath = path
	e.content = string(data)
	e.language = DetectLanguage(path)
	e.highlighter = NewHighlighter(e.language)
	e.highlighter.SetContent(e.content)

	return nil
}

// GetHighlighted returns the highlighted version of the current content
func (e *Editor) GetHighlighted() string {
	if e.highlighter == nil {
		return e.content
	}
	return e.highlighter.Highlight()
}

// Suggest provides autocomplete suggestions at the cursor position
func (e *Editor) Suggest(cursorLine, cursorCol int) []string {
	lines := strings.Split(e.content, "\n")
	if cursorLine < 0 || cursorLine >= len(lines) {
		return []string{}
	}
	return e.assist.SuggestAutocomplete(lines[cursorLine], cursorCol)
}

// Explain generates an explanation for a code range
func (e *Editor) Explain(startLine, endLine int) string {
	return e.assist.ExplainCode(startLine, endLine, e.content)
}

// FixError suggests fixes for errors on a specific line
func (e *Editor) FixError(lineNum int) string {
	lines := strings.Split(e.content, "\n")
	if lineNum < 0 || lineNum >= len(lines) {
		return "Invalid line number"
	}
	return e.assist.FixError(lines[lineNum])
}

// Content returns the current content of the editor
func (e *Editor) Content() string {
	return e.content
}

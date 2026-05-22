package gui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const (
	DefaultEditorFontSize = 13
	DefaultTerminalID     = "terminal-1"
)

// SuggestionStatus is the review state for an inline AI edit suggestion.
type SuggestionStatus int

const (
	SuggestionPending SuggestionStatus = iota
	SuggestionAccepted
	SuggestionRejected
)

// FileNode is one row in the Mini IDE project tree.
type FileNode struct {
	Name     string
	Path     string
	IsDir    bool
	Expanded bool
	Language string
	Size     int64
	Children []FileNode
}

// EditorTab is the GUI-facing state for one open file.
type EditorTab struct {
	Path           string
	Name           string
	Content        string
	Language       string
	Dirty          bool
	CursorLine     int
	CursorColumn   int
	SoftWrap       bool
	FontSize       int
	MinimapEnabled bool
}

// AISuggestion represents one inline AI suggestion shown inside the editor.
type AISuggestion struct {
	ID          string
	Path        string
	StartLine   int
	EndLine     int
	Replacement string
	Explanation string
	Status      SuggestionStatus
}

// TerminalPane is the view-model for one embedded terminal split.
type TerminalPane struct {
	ID        string
	Title     string
	CWD       string
	Shell     string
	Collapsed bool
	Active    bool
	Lines     []string
}

// ExternalEditor describes one configured external editor command.
type ExternalEditor struct {
	Name      string
	Command   string
	Args      []string
	Icon      string
	FileTypes []string
}

// ExternalLaunch is the concrete command line the native shell should run.
type ExternalLaunch struct {
	Name    string
	Command string
	Args    []string
}

// MiniIDE is the GUI view-model for PRD §11.3. It owns the state the Tauri
// frontend needs for file browsing, code tabs, inline AI suggestions, terminal
// splits, and external-editor launch commands. It does not execute shell or
// editor processes; platform adapters consume the launch specs.
type MiniIDE struct {
	mu sync.RWMutex

	root          string
	activeTab     string
	tabs          map[string]*EditorTab
	tabOrder      []string
	suggestions   map[string]*AISuggestion
	terminalOrder []string
	terminals     map[string]*TerminalPane
	editors       []ExternalEditor
	visible       bool
}

// NewMiniIDE creates an empty Mini IDE model scoped to root.
func NewMiniIDE(root string) (*MiniIDE, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mini ide root %q is not a directory", abs)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return &MiniIDE{
		root:        filepath.Clean(abs),
		tabs:        make(map[string]*EditorTab),
		suggestions: make(map[string]*AISuggestion),
		terminals: map[string]*TerminalPane{
			DefaultTerminalID: {
				ID:     DefaultTerminalID,
				Title:  "Shell",
				CWD:    filepath.Clean(abs),
				Shell:  shell,
				Active: true,
			},
		},
		terminalOrder: []string{DefaultTerminalID},
		editors:       DefaultExternalEditors(),
	}, nil
}

// DefaultExternalEditors returns the built-in external editor options from
// PRD §11.3. User settings can replace these through SetExternalEditors.
func DefaultExternalEditors() []ExternalEditor {
	return []ExternalEditor{
		{Name: "VS Code", Command: "code", Args: []string{"{file}"}, Icon: "vscode", FileTypes: []string{"*"}},
		{Name: "Obsidian", Command: "open", Args: []string{"obsidian://open?file={file}"}, Icon: "obsidian", FileTypes: []string{".md", ".markdown"}},
		{Name: "Xcode", Command: "open", Args: []string{"-a", "Xcode", "{file}"}, Icon: "xcode", FileTypes: []string{".swift", ".xcodeproj", ".xcworkspace"}},
	}
}

// Root returns the absolute project root for this IDE session.
func (m *MiniIDE) Root() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.root
}

func (m *MiniIDE) Show() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.visible = true
}

func (m *MiniIDE) Hide() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.visible = false
}

func (m *MiniIDE) Visible() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.visible
}

// BuildFileTree returns a deterministic snapshot of root. maxDepth and
// maxEntries protect the GUI from huge worktrees; values <= 0 mean unlimited.
func (m *MiniIDE) BuildFileTree(maxDepth, maxEntries int) (FileNode, error) {
	m.mu.RLock()
	root := m.root
	m.mu.RUnlock()

	count := 0
	return buildFileNode(root, root, 0, maxDepth, maxEntries, &count)
}

// OpenFile reads path, opens or refreshes its tab, and makes the IDE visible.
func (m *MiniIDE) OpenFile(path string) (*EditorTab, error) {
	abs, err := m.resolvePath(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("cannot open directory %q as file", abs)
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tab := &EditorTab{
		Path:           abs,
		Name:           filepath.Base(abs),
		Content:        string(content),
		Language:       DetectLanguage(abs),
		CursorLine:     1,
		CursorColumn:   1,
		SoftWrap:       true,
		FontSize:       DefaultEditorFontSize,
		MinimapEnabled: info.Size() >= 64*1024,
	}
	if existing, ok := m.tabs[abs]; ok {
		tab.CursorLine = existing.CursorLine
		tab.CursorColumn = existing.CursorColumn
		tab.SoftWrap = existing.SoftWrap
		tab.FontSize = existing.FontSize
		tab.MinimapEnabled = existing.MinimapEnabled
	} else {
		m.tabOrder = append(m.tabOrder, abs)
	}
	m.tabs[abs] = tab
	m.activeTab = abs
	m.visible = true

	cp := *tab
	return &cp, nil
}

// UpdateBuffer replaces a tab's in-memory content and marks it dirty.
func (m *MiniIDE) UpdateBuffer(path, content string) error {
	abs, err := m.resolvePath(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tab, ok := m.tabs[abs]
	if !ok {
		return fmt.Errorf("file %q is not open", abs)
	}
	tab.Content = content
	tab.Dirty = true
	return nil
}

// Save writes the current buffer to disk and clears the dirty bit.
func (m *MiniIDE) Save(path string) error {
	abs, err := m.resolvePath(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tab, ok := m.tabs[abs]
	if !ok {
		return fmt.Errorf("file %q is not open", abs)
	}
	if err := os.WriteFile(abs, []byte(tab.Content), 0o644); err != nil {
		return err
	}
	tab.Dirty = false
	return nil
}

// MoveCursor updates the active cursor position for line-aware UI affordances.
func (m *MiniIDE) MoveCursor(path string, line, column int) error {
	if line < 1 || column < 1 {
		return errors.New("cursor line and column are 1-based")
	}
	abs, err := m.resolvePath(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tab, ok := m.tabs[abs]
	if !ok {
		return fmt.Errorf("file %q is not open", abs)
	}
	tab.CursorLine = line
	tab.CursorColumn = column
	return nil
}

func (m *MiniIDE) SetSoftWrap(path string, enabled bool) error {
	return m.updateTab(path, func(tab *EditorTab) { tab.SoftWrap = enabled })
}

func (m *MiniIDE) SetFontSize(path string, size int) error {
	if size < 9 || size > 32 {
		return fmt.Errorf("font size %d outside supported range 9..32", size)
	}
	return m.updateTab(path, func(tab *EditorTab) { tab.FontSize = size })
}

func (m *MiniIDE) SetMinimap(path string, enabled bool) error {
	return m.updateTab(path, func(tab *EditorTab) { tab.MinimapEnabled = enabled })
}

func (m *MiniIDE) ActiveTab() *EditorTab {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeTab == "" {
		return nil
	}
	return cloneTab(m.tabs[m.activeTab])
}

func (m *MiniIDE) Tabs() []EditorTab {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]EditorTab, 0, len(m.tabOrder))
	for _, path := range m.tabOrder {
		out = append(out, *cloneTab(m.tabs[path]))
	}
	return out
}

func (m *MiniIDE) CloseTab(path string) error {
	abs, err := m.resolvePath(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tabs[abs]; !ok {
		return fmt.Errorf("file %q is not open", abs)
	}
	delete(m.tabs, abs)
	for i, p := range m.tabOrder {
		if p == abs {
			m.tabOrder = append(m.tabOrder[:i], m.tabOrder[i+1:]...)
			break
		}
	}
	if m.activeTab == abs {
		m.activeTab = ""
		if len(m.tabOrder) > 0 {
			m.activeTab = m.tabOrder[len(m.tabOrder)-1]
		}
	}
	return nil
}

// Completions returns keyword and in-buffer symbol completions for prefix.
func (m *MiniIDE) Completions(path, prefix string, limit int) ([]string, error) {
	abs, err := m.resolvePath(path)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	tab, ok := m.tabs[abs]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("file %q is not open", abs)
	}
	content := tab.Content
	lang := tab.Language
	m.mu.RUnlock()

	seen := make(map[string]bool)
	var out []string
	add := func(candidate string) {
		if candidate == "" || seen[candidate] || !strings.HasPrefix(candidate, prefix) {
			return
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	for _, kw := range languageKeywords(lang) {
		add(kw)
	}
	for _, sym := range symbols(content) {
		add(sym)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MiniIDE) AddSuggestion(s AISuggestion) error {
	if s.ID == "" {
		return errors.New("suggestion ID is required")
	}
	abs, err := m.resolvePath(s.Path)
	if err != nil {
		return err
	}
	if s.StartLine < 1 || s.EndLine < s.StartLine {
		return errors.New("suggestion line range is invalid")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s.Path = abs
	s.Status = SuggestionPending
	cp := s
	m.suggestions[s.ID] = &cp
	return nil
}

func (m *MiniIDE) ResolveSuggestion(id string, status SuggestionStatus) error {
	if status == SuggestionPending {
		return errors.New("resolved suggestion status must be accepted or rejected")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.suggestions[id]
	if !ok {
		return fmt.Errorf("suggestion %q not found", id)
	}
	s.Status = status
	return nil
}

func (m *MiniIDE) SuggestionsForFile(path string) ([]AISuggestion, error) {
	abs, err := m.resolvePath(path)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []AISuggestion
	for _, s := range m.suggestions {
		if s.Path == abs {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *MiniIDE) AddTerminal(id, title, cwd, shell string) error {
	if id == "" {
		return errors.New("terminal ID is required")
	}
	abs, err := m.resolvePath(cwd)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("terminal cwd %q is not a directory", abs)
	}
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.terminals[id]; !exists {
		m.terminalOrder = append(m.terminalOrder, id)
	}
	for _, pane := range m.terminals {
		pane.Active = false
	}
	m.terminals[id] = &TerminalPane{ID: id, Title: title, CWD: abs, Shell: shell, Active: true}
	return nil
}

func (m *MiniIDE) AppendTerminalOutput(id string, lines ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pane, ok := m.terminals[id]
	if !ok {
		return fmt.Errorf("terminal %q not found", id)
	}
	pane.Lines = append(pane.Lines, lines...)
	return nil
}

func (m *MiniIDE) SetTerminalCollapsed(id string, collapsed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pane, ok := m.terminals[id]
	if !ok {
		return fmt.Errorf("terminal %q not found", id)
	}
	pane.Collapsed = collapsed
	return nil
}

func (m *MiniIDE) Terminals() []TerminalPane {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]TerminalPane, 0, len(m.terminalOrder))
	for _, id := range m.terminalOrder {
		cp := *m.terminals[id]
		cp.Lines = append([]string(nil), cp.Lines...)
		out = append(out, cp)
	}
	return out
}

func (m *MiniIDE) SetExternalEditors(editors []ExternalEditor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.editors = append([]ExternalEditor(nil), editors...)
}

func (m *MiniIDE) ExternalLaunches(path string) ([]ExternalLaunch, error) {
	abs, err := m.resolvePath(path)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	editors := append([]ExternalEditor(nil), m.editors...)
	m.mu.RUnlock()

	var out []ExternalLaunch
	for _, editor := range editors {
		if !editorSupportsFile(editor, abs) {
			continue
		}
		args := make([]string, len(editor.Args))
		for i, arg := range editor.Args {
			args[i] = strings.ReplaceAll(arg, "{file}", abs)
		}
		out = append(out, ExternalLaunch{Name: editor.Name, Command: editor.Command, Args: args})
	}
	return out, nil
}

func (m *MiniIDE) updateTab(path string, fn func(*EditorTab)) error {
	abs, err := m.resolvePath(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tab, ok := m.tabs[abs]
	if !ok {
		return fmt.Errorf("file %q is not open", abs)
	}
	fn(tab)
	return nil
}

func (m *MiniIDE) resolvePath(path string) (string, error) {
	m.mu.RLock()
	root := m.root
	m.mu.RUnlock()

	if path == "" {
		return "", errors.New("path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q escapes mini ide root %q", abs, root)
	}
	return abs, nil
}

func buildFileNode(root, path string, depth, maxDepth, maxEntries int, count *int) (FileNode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileNode{}, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return FileNode{}, err
	}
	if rel == "." {
		rel = ""
	}
	node := FileNode{
		Name:     filepath.Base(path),
		Path:     rel,
		IsDir:    info.IsDir(),
		Expanded: info.IsDir() && (maxDepth <= 0 || depth < maxDepth),
		Language: DetectLanguage(path),
		Size:     info.Size(),
	}
	if !info.IsDir() || (maxDepth > 0 && depth >= maxDepth) {
		return node, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return FileNode{}, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if maxEntries > 0 && *count >= maxEntries {
			break
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		(*count)++
		child, err := buildFileNode(root, filepath.Join(path, entry.Name()), depth+1, maxDepth, maxEntries, count)
		if err != nil {
			return FileNode{}, err
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}

// DetectLanguage returns a stable language ID for syntax highlighting adapters.
func DetectLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".swift":
		return "swift"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".md", ".markdown":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".sh", ".bash", ".zsh":
		return "shell"
	default:
		return "plaintext"
	}
}

func languageKeywords(language string) []string {
	switch language {
	case "go":
		return []string{"break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for", "func", "go", "if", "import", "interface", "map", "package", "range", "return", "select", "struct", "switch", "type", "var"}
	case "typescript", "javascript":
		return []string{"async", "await", "break", "case", "catch", "class", "const", "continue", "default", "else", "export", "extends", "finally", "for", "function", "if", "import", "interface", "let", "return", "switch", "try", "type", "var"}
	case "python":
		return []string{"and", "as", "async", "await", "break", "class", "continue", "def", "elif", "else", "except", "finally", "for", "from", "if", "import", "in", "lambda", "return", "try", "while", "with", "yield"}
	default:
		return nil
	}
}

func symbols(content string) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	})
	var out []string
	for _, field := range fields {
		if field == "" || unicode.IsDigit(rune(field[0])) {
			continue
		}
		out = append(out, field)
	}
	return out
}

func editorSupportsFile(editor ExternalEditor, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, fileType := range editor.FileTypes {
		ft := strings.ToLower(fileType)
		if ft == "*" || ft == ext {
			return true
		}
		if strings.HasSuffix(ft, string(os.PathSeparator)) && strings.HasPrefix(path, ft) {
			return true
		}
	}
	return false
}

func cloneTab(tab *EditorTab) *EditorTab {
	if tab == nil {
		return nil
	}
	cp := *tab
	return &cp
}

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// MemoryKind and MemoryEntry are canonical in contracts; aliased here so that
// existing callers within the core package compile without changes.
type MemoryKind = contracts.MemoryKind
type MemoryEntry = contracts.MemoryEntry

const (
	MemoryKindShortTerm         = contracts.MemoryKindShortTerm
	MemoryKindLongTermEpisodic  = contracts.MemoryKindLongTermEpisodic
	MemoryKindSkill             = contracts.MemoryKindSkill
	defaultConduitDirectoryName = ".conduit"
)

// IdentityConfig locates the static identity files and the flat-file memory
// store. The default layout is ~/.conduit/{SOUL.md,USER.md,memory/*.md}.
type IdentityConfig struct {
	HomeDir   string
	SoulPath  string
	UserPath  string
	MemoryDir string
}

// DefaultIdentityConfig returns the local-first identity layout for this user.
func DefaultIdentityConfig() IdentityConfig {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}

	conduitHome := filepath.Join(home, defaultConduitDirectoryName)
	return IdentityConfig{
		HomeDir:   conduitHome,
		SoulPath:  filepath.Join(conduitHome, "SOUL.md"),
		UserPath:  filepath.Join(conduitHome, "USER.md"),
		MemoryDir: filepath.Join(conduitHome, "memory"),
	}
}

func (c IdentityConfig) withDefaults() IdentityConfig {
	if c.HomeDir == "" {
		c.HomeDir = DefaultIdentityConfig().HomeDir
	}
	if c.SoulPath == "" {
		c.SoulPath = filepath.Join(c.HomeDir, "SOUL.md")
	}
	if c.UserPath == "" {
		c.UserPath = filepath.Join(c.HomeDir, "USER.md")
	}
	if c.MemoryDir == "" {
		c.MemoryDir = filepath.Join(c.HomeDir, "memory")
	}
	return c
}

// IdentitySnapshot is the loaded three-layer identity state for prompt assembly.
type IdentitySnapshot struct {
	Soul       string
	User       string
	ShortTerm  []MemoryEntry
	LongTerm   []MemoryEntry
	Skill      []MemoryEntry
	LoadedFrom IdentityConfig
}

// SystemPrompt composes identity context in increasing priority. SOUL.md is
// injected last so it can override USER.md and memory context.
func (s IdentitySnapshot) SystemPrompt() string {
	var sections []string

	memory := formatMemorySections(s.ShortTerm, s.LongTerm, s.Skill)
	if memory != "" {
		sections = append(sections, "## Memory\n\n"+memory)
	}
	if strings.TrimSpace(s.User) != "" {
		sections = append(sections, "## USER.md\n\n"+strings.TrimSpace(s.User))
	}
	if strings.TrimSpace(s.Soul) != "" {
		sections = append(sections, "## SOUL.md\n\n"+strings.TrimSpace(s.Soul))
	}

	return strings.Join(sections, "\n\n")
}

// IdentityManager owns the static and dynamic identity layers for one engine.
type IdentityManager struct {
	config    IdentityConfig
	shortTerm []MemoryEntry
	now       func() time.Time
}

// NewIdentityManager creates an identity manager using the provided config.
func NewIdentityManager(config IdentityConfig) *IdentityManager {
	return &IdentityManager{
		config: config.withDefaults(),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Config returns the resolved identity paths.
func (m *IdentityManager) Config() IdentityConfig {
	return m.config
}

// RememberShortTerm records session-scoped context that is never written to disk.
func (m *IdentityManager) RememberShortTerm(title, body string) MemoryEntry {
	entry := MemoryEntry{
		ID:        memoryID(m.now(), title),
		Kind:      MemoryKindShortTerm,
		Title:     strings.TrimSpace(title),
		Body:      strings.TrimSpace(body),
		CreatedAt: m.now(),
	}
	m.shortTerm = append(m.shortTerm, entry)
	return entry
}

// ClearShortTerm drops session-scoped memory at session end.
func (m *IdentityManager) ClearShortTerm() {
	m.shortTerm = nil
}

// WriteMemory persists a long-term episodic or skill memory as a flat markdown file.
func (m *IdentityManager) WriteMemory(kind MemoryKind, title, body string) (MemoryEntry, error) {
	if kind == MemoryKindShortTerm {
		return MemoryEntry{}, fmt.Errorf("short-term memory is session-scoped; use RememberShortTerm")
	}
	if kind == "" {
		kind = MemoryKindLongTermEpisodic
	}

	createdAt := m.now()
	entry := MemoryEntry{
		ID:        memoryID(createdAt, title),
		Kind:      kind,
		Title:     strings.TrimSpace(title),
		Body:      strings.TrimSpace(body),
		CreatedAt: createdAt,
	}
	if entry.Title == "" {
		entry.Title = "Untitled memory"
	}

	if err := os.MkdirAll(m.config.MemoryDir, 0o755); err != nil {
		return MemoryEntry{}, err
	}

	entry.Path = filepath.Join(m.config.MemoryDir, entry.ID+".md")
	contents := fmt.Sprintf("---\nkind: %s\ncreated_at: %s\ntitle: %s\n---\n\n# %s\n\n%s\n",
		entry.Kind,
		entry.CreatedAt.Format(time.RFC3339),
		entry.Title,
		entry.Title,
		entry.Body,
	)

	if err := os.WriteFile(entry.Path, []byte(contents), 0o644); err != nil {
		return MemoryEntry{}, err
	}
	return entry, nil
}

// Load reads SOUL.md, USER.md, and all flat markdown files from the memory dir.
func (m *IdentityManager) Load() (IdentitySnapshot, error) {
	if err := os.MkdirAll(m.config.MemoryDir, 0o755); err != nil {
		return IdentitySnapshot{}, err
	}

	soul, err := readOptionalMarkdown(m.config.SoulPath)
	if err != nil {
		return IdentitySnapshot{}, err
	}
	user, err := readOptionalMarkdown(m.config.UserPath)
	if err != nil {
		return IdentitySnapshot{}, err
	}

	longTerm, skill, err := m.loadMemoryFiles()
	if err != nil {
		return IdentitySnapshot{}, err
	}

	return IdentitySnapshot{
		Soul:       soul,
		User:       user,
		ShortTerm:  append([]MemoryEntry(nil), m.shortTerm...),
		LongTerm:   longTerm,
		Skill:      skill,
		LoadedFrom: m.config,
	}, nil
}

func (m *IdentityManager) loadMemoryFiles() ([]MemoryEntry, []MemoryEntry, error) {
	files, err := os.ReadDir(m.config.MemoryDir)
	if err != nil {
		return nil, nil, err
	}

	var entries []MemoryEntry
	for _, file := range files {
		if file.IsDir() || strings.ToLower(filepath.Ext(file.Name())) != ".md" {
			continue
		}

		path := filepath.Join(m.config.MemoryDir, file.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}

		entry := parseMemoryMarkdown(path, strings.TrimSuffix(file.Name(), ".md"), string(contents))
		info, err := file.Info()
		if err == nil && entry.CreatedAt.IsZero() {
			entry.CreatedAt = info.ModTime().UTC()
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})

	var longTerm []MemoryEntry
	var skill []MemoryEntry
	for _, entry := range entries {
		switch entry.Kind {
		case MemoryKindSkill:
			skill = append(skill, entry)
		default:
			if entry.Kind == "" {
				entry.Kind = MemoryKindLongTermEpisodic
			}
			longTerm = append(longTerm, entry)
		}
	}

	return longTerm, skill, nil
}

func readOptionalMarkdown(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSpace(string(contents)), nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	return "", err
}

func parseMemoryMarkdown(path, id, contents string) MemoryEntry {
	entry := MemoryEntry{
		ID:   id,
		Kind: MemoryKindLongTermEpisodic,
		Path: path,
		Body: strings.TrimSpace(contents),
	}

	if !strings.HasPrefix(contents, "---\n") {
		entry.Title = firstMarkdownHeading(contents)
		return entry
	}

	end := strings.Index(contents[len("---\n"):], "\n---")
	if end == -1 {
		entry.Title = firstMarkdownHeading(contents)
		return entry
	}

	frontMatter := contents[len("---\n") : len("---\n")+end]
	body := strings.TrimSpace(contents[len("---\n")+end+len("\n---"):])
	entry.Body = body
	for _, line := range strings.Split(frontMatter, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "kind":
			entry.Kind = MemoryKind(value)
		case "created_at":
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				entry.CreatedAt = parsed.UTC()
			}
		case "title":
			entry.Title = value
		}
	}
	if entry.Title == "" {
		entry.Title = firstMarkdownHeading(body)
	}
	return entry
}

func firstMarkdownHeading(contents string) string {
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return ""
}

func formatMemorySections(shortTerm, longTerm, skill []MemoryEntry) string {
	sections := []struct {
		title   string
		entries []MemoryEntry
	}{
		{"Short-term context", shortTerm},
		{"Long-term episodic memory", longTerm},
		{"Skill memory", skill},
	}

	var out []string
	for _, section := range sections {
		if len(section.entries) == 0 {
			continue
		}

		lines := []string{"### " + section.title}
		for _, entry := range section.entries {
			title := entry.Title
			if title == "" {
				title = entry.ID
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", title, oneLine(entry.Body)))
		}
		out = append(out, strings.Join(lines, "\n"))
	}

	return strings.Join(out, "\n\n")
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func memoryID(createdAt time.Time, title string) string {
	slug := slugify(title)
	if slug == "" {
		slug = "memory"
	}
	return createdAt.UTC().Format("20060102T150405Z") + "-" + slug
}

func slugify(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

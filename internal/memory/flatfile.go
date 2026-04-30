package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const defaultMemoryDir = ".conduit/memory"

// FlatFileProvider stores memory entries as markdown files under a configurable
// directory. Files are Spotlight-compatible and can be read back without a DB.
type FlatFileProvider struct {
	dir string
	now func() time.Time
}

// NewFlatFileProvider creates a provider writing to dir.
// An empty dir falls back to ~/.conduit/memory at Initialize time.
func NewFlatFileProvider(dir string) *FlatFileProvider {
	return &FlatFileProvider{
		dir: dir,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (p *FlatFileProvider) Initialize(_ context.Context, cfg contracts.MemoryConfig) error {
	if cfg.Dir != "" {
		p.dir = cfg.Dir
	}
	if p.dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("memory: resolve home dir: %w", err)
		}
		p.dir = filepath.Join(home, defaultMemoryDir)
	}
	return os.MkdirAll(p.dir, 0o755)
}

func (p *FlatFileProvider) Prefetch(_ context.Context, _ string) ([]contracts.MemoryEntry, error) {
	return p.readAll()
}

func (p *FlatFileProvider) Write(_ context.Context, entry contracts.MemoryEntry) error {
	if entry.Kind == contracts.MemoryKindShortTerm {
		return fmt.Errorf("memory: short-term entries are session-scoped and must not be persisted")
	}
	if entry.Kind == "" {
		entry.Kind = contracts.MemoryKindLongTermEpisodic
	}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = p.now()
	}
	if entry.ID == "" {
		entry.ID = entryID(entry.CreatedAt, entry.Title)
	}
	if entry.Title == "" {
		entry.Title = "Untitled memory"
	}
	path := filepath.Join(p.dir, entry.ID+".md")
	contents := fmt.Sprintf("---\nkind: %s\ncreated_at: %s\ntitle: %s\n---\n\n# %s\n\n%s\n",
		entry.Kind,
		entry.CreatedAt.Format(time.RFC3339),
		entry.Title,
		entry.Title,
		entry.Body,
	)
	return os.WriteFile(path, []byte(contents), 0o644)
}

// Search returns entries whose title or body contains query (case-insensitive).
// Results are returned newest-first and capped at limit (0 = no cap).
func (p *FlatFileProvider) Search(_ context.Context, query string, limit int) ([]contracts.MemoryEntry, error) {
	entries, err := p.readAll()
	if err != nil {
		return nil, err
	}

	// Reverse so newest entries surface first in results.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	if strings.TrimSpace(query) == "" {
		if limit > 0 && len(entries) > limit {
			return entries[:limit], nil
		}
		return entries, nil
	}

	q := strings.ToLower(query)
	var results []contracts.MemoryEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Title), q) || strings.Contains(strings.ToLower(e.Body), q) {
			results = append(results, e)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// Compress is intentionally minimal for the flat-file provider — semantic
// compression requires a model call and belongs in a future provider.
func (p *FlatFileProvider) Compress(_ context.Context) (*contracts.CompressedContext, error) {
	return nil, nil
}

func (p *FlatFileProvider) Shutdown(_ context.Context) error { return nil }

// Dir returns the resolved storage directory (available after Initialize).
func (p *FlatFileProvider) Dir() string { return p.dir }

func (p *FlatFileProvider) readAll() ([]contracts.MemoryEntry, error) {
	files, err := os.ReadDir(p.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []contracts.MemoryEntry
	for _, f := range files {
		if f.IsDir() || strings.ToLower(filepath.Ext(f.Name())) != ".md" {
			continue
		}
		path := filepath.Join(p.dir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		entry := parseMemoryFile(path, strings.TrimSuffix(f.Name(), ".md"), string(data))
		if entry.CreatedAt.IsZero() {
			if info, err := f.Info(); err == nil {
				entry.CreatedAt = info.ModTime().UTC()
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
	return entries, nil
}

func parseMemoryFile(path, id, contents string) contracts.MemoryEntry {
	entry := contracts.MemoryEntry{
		ID:   id,
		Kind: contracts.MemoryKindLongTermEpisodic,
		Path: path,
		Body: strings.TrimSpace(contents),
	}
	if !strings.HasPrefix(contents, "---\n") {
		entry.Title = firstHeading(contents)
		return entry
	}
	end := strings.Index(contents[len("---\n"):], "\n---")
	if end == -1 {
		entry.Title = firstHeading(contents)
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
			entry.Kind = contracts.MemoryKind(value)
		case "created_at":
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				entry.CreatedAt = parsed.UTC()
			}
		case "title":
			entry.Title = value
		}
	}
	if entry.Title == "" {
		entry.Title = firstHeading(body)
	}
	return entry
}

func firstHeading(contents string) string {
	for _, line := range strings.Split(contents, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}

func entryID(createdAt time.Time, title string) string {
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
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

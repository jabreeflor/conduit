package memory

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const defaultMemoryDir = ".conduit/memory"

// FlatFileProvider is the default Provider. Each Entry is stored as a
// Spotlight-indexed Markdown file under ~/.conduit/memory/. macOS Spotlight
// indexes .md files in user home directories automatically, so both the YAML
// frontmatter fields and the prose body are full-text searchable without any
// additional integration.
type FlatFileProvider struct {
	mu  sync.RWMutex
	dir string
}

// NewFlatFileProvider creates a FlatFileProvider writing to ~/.conduit/memory/.
func NewFlatFileProvider() (*FlatFileProvider, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("memory: resolve home dir: %w", err)
	}
	return &FlatFileProvider{dir: filepath.Join(home, defaultMemoryDir)}, nil
}

// NewFlatFileProviderAt creates a FlatFileProvider writing to an explicit
// directory (useful in tests).
func NewFlatFileProviderAt(dir string) *FlatFileProvider {
	return &FlatFileProvider{dir: dir}
}

// Initialize creates the memory directory if absent.
func (p *FlatFileProvider) Initialize(_ context.Context) error {
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return fmt.Errorf("memory: create dir: %w", err)
	}
	return nil
}

// Prefetch delegates to Search — the flat-file backend has no warm cache.
func (p *FlatFileProvider) Prefetch(ctx context.Context, query string) ([]Entry, error) {
	return p.Search(ctx, query)
}

// Write persists an Entry as a Markdown file with YAML frontmatter.
// If an entry with the same ID already exists its file is replaced in-place,
// which handles title renames cleanly.
func (p *FlatFileProvider) Write(_ context.Context, entry Entry) error {
	if entry.ID == "" {
		entry.ID = generateID()
	}
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return fmt.Errorf("memory: create dir: %w", err)
	}

	// Remove any stale file that already holds this ID (title may have changed).
	if old, ok := p.findFileByID(entry.ID); ok {
		os.Remove(old) //nolint:errcheck
	}

	return os.WriteFile(p.pathFor(entry), []byte(marshalEntry(entry)), 0o644)
}

// Search scans all .md files and returns entries whose title, body, or tags
// contain query (case-insensitive). An empty query returns everything.
func (p *FlatFileProvider) Search(_ context.Context, query string) ([]Entry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entries, err := p.readAll()
	if err != nil {
		return nil, err
	}
	if query == "" {
		return entries, nil
	}
	q := strings.ToLower(query)
	out := entries[:0]
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Title), q) ||
			strings.Contains(strings.ToLower(e.Body), q) ||
			containsTag(e.Tags, q) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Delete removes the entry with the given ID. Idempotent: returns nil if no
// matching file is present.
func (p *FlatFileProvider) Delete(_ context.Context, id string) error {
	if id == "" {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if path, ok := p.findFileByID(id); ok {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("memory: delete %s: %w", path, err)
		}
	}
	return nil
}

// Prune removes every entry matched by query (case-insensitive substring on
// title/body/tags) except those with Pinned=true. An empty query prunes all
// non-pinned entries. Returns the IDs that were removed.
func (p *FlatFileProvider) Prune(_ context.Context, query string) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entries, err := p.readAll()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var removed []string
	for _, e := range entries {
		if e.Pinned {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(e.Title), q) &&
				!strings.Contains(strings.ToLower(e.Body), q) &&
				!containsTag(e.Tags, q) {
				continue
			}
		}
		path, ok := p.findFileByID(e.ID)
		if !ok {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("memory: prune %s: %w", path, err)
		}
		removed = append(removed, e.ID)
	}
	return removed, nil
}

// Compress is a no-op for the flat-file backend. Entry curation is agent-driven
// and happens after task completion via Write calls.
func (p *FlatFileProvider) Compress(_ context.Context) error { return nil }

// Shutdown is a no-op — all writes are synchronous.
func (p *FlatFileProvider) Shutdown(_ context.Context) error { return nil }

// ── internal helpers ──────────────────────────────────────────────────────────

func (p *FlatFileProvider) readAll() ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(p.dir, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("memory: glob: %w", err)
	}
	out := make([]Entry, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if e, ok := parseEntry(string(data)); ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// findFileByID returns the path of the first .md file whose frontmatter id
// matches. Must be called with p.mu held (at least read).
func (p *FlatFileProvider) findFileByID(id string) (string, bool) {
	suffix := "-" + shortID(id) + ".md"
	files, _ := filepath.Glob(filepath.Join(p.dir, "*"+suffix))
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if e, ok := parseEntry(string(data)); ok && e.ID == id {
			return f, true
		}
	}
	return "", false
}

func (p *FlatFileProvider) pathFor(e Entry) string {
	slug := slugify(e.Title)
	if slug == "" {
		slug = "entry"
	}
	return filepath.Join(p.dir, slug+"-"+shortID(e.ID)+".md")
}

// marshalEntry renders an Entry as Markdown with YAML frontmatter.
// Spotlight indexes both the frontmatter text and the body automatically.
func marshalEntry(e Entry) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", e.ID)
	fmt.Fprintf(&b, "kind: %s\n", e.Kind)
	fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(e.Tags, ", "))
	fmt.Fprintf(&b, "created: %s\n", e.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "updated: %s\n", e.UpdatedAt.Format(time.RFC3339))
	if e.Pinned {
		// Only emit when true so existing files stay byte-identical.
		b.WriteString("pinned: true\n")
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", e.Title)
	b.WriteString(strings.TrimRight(e.Body, "\n"))
	b.WriteByte('\n')
	return b.String()
}

// parseEntry extracts an Entry from a Markdown string with YAML frontmatter.
func parseEntry(s string) (Entry, bool) {
	if !strings.HasPrefix(s, "---\n") {
		return Entry{}, false
	}
	rest := s[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return Entry{}, false
	}
	fm := rest[:end]
	body := strings.TrimPrefix(rest[end+5:], "\n")

	var title, bodyText string
	if strings.HasPrefix(body, "# ") {
		if nl := strings.Index(body, "\n"); nl > 0 {
			title = strings.TrimPrefix(body[:nl], "# ")
			bodyText = strings.TrimPrefix(body[nl+1:], "\n")
		}
	}

	e := Entry{Title: title, Body: bodyText}
	scanner := bufio.NewScanner(strings.NewReader(fm))
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), ": ")
		if !ok {
			continue
		}
		switch k {
		case "id":
			e.ID = v
		case "kind":
			e.Kind = Kind(v)
		case "tags":
			e.Tags = parseTags(v)
		case "created":
			e.CreatedAt, _ = time.Parse(time.RFC3339, v)
		case "updated":
			e.UpdatedAt, _ = time.Parse(time.RFC3339, v)
		case "pinned":
			e.Pinned = strings.EqualFold(strings.TrimSpace(v), "true")
		}
	}
	if e.ID == "" {
		return Entry{}, false
	}
	return e, true
}

func parseTags(s string) []string {
	s = strings.Trim(s, "[]")
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

// generateID returns a 20-char hex string: 16 chars of timestamp + 4 of random.
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b) //nolint:errcheck
	return fmt.Sprintf("%016x%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// shortID returns the first 8 chars of an ID for use in filenames.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func containsTag(tags []string, q string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

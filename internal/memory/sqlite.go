package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const defaultSQLitePath = ".conduit/memory/memory.db"

// SQLiteProvider stores entries in a single SQLite database with an FTS5
// virtual table for tokenized full-text search (PRD §6.4: "structured recall
// with full-text search").
//
// Compared to FlatFileProvider it is faster on large stores and supports
// phrase / prefix queries, but its data is opaque to Spotlight. Pick the one
// that matches the user's recall workflow.
type SQLiteProvider struct {
	mu   sync.Mutex
	path string
	db   *sql.DB
}

// NewSQLiteProvider creates a provider writing to ~/.conduit/memory/memory.db.
func NewSQLiteProvider() (*SQLiteProvider, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("memory: resolve home dir: %w", err)
	}
	return &SQLiteProvider{path: filepath.Join(home, defaultSQLitePath)}, nil
}

// NewSQLiteProviderAt creates a provider writing to an explicit file path
// (useful in tests). Pass ":memory:" for an in-memory database.
func NewSQLiteProviderAt(path string) *SQLiteProvider {
	return &SQLiteProvider{path: path}
}

// Initialize opens the database and creates the schema if absent. Safe to call
// multiple times.
func (p *SQLiteProvider) Initialize(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil {
		return nil
	}
	if p.path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
			return fmt.Errorf("memory: create dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", p.path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return fmt.Errorf("memory: open sqlite: %w", err)
	}
	// modernc's sqlite driver handles its own concurrency, but FTS5 triggers
	// can deadlock under high parallel writes — keep the pool serialised.
	db.SetMaxOpenConns(1)
	if err := initSchema(db); err != nil {
		_ = db.Close()
		return err
	}
	p.db = db
	return nil
}

// Prefetch delegates to Search (no separate warm cache; FTS lookups are fast).
func (p *SQLiteProvider) Prefetch(ctx context.Context, query string) ([]Entry, error) {
	return p.Search(ctx, query)
}

// Write inserts or updates an Entry. ID is generated when empty; CreatedAt is
// preserved on update; UpdatedAt is always refreshed.
func (p *SQLiteProvider) Write(ctx context.Context, entry Entry) error {
	if err := p.ensureOpen(); err != nil {
		return err
	}
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

	pinned := 0
	if entry.Pinned {
		pinned = 1
	}
	tags := strings.Join(entry.Tags, ",")

	_, err := p.db.ExecContext(ctx, `
		INSERT INTO entries (id, kind, title, body, tags, created_at, updated_at, pinned)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind=excluded.kind,
			title=excluded.title,
			body=excluded.body,
			tags=excluded.tags,
			updated_at=excluded.updated_at,
			pinned=excluded.pinned
	`, entry.ID, string(entry.Kind), entry.Title, entry.Body, tags,
		entry.CreatedAt.Format(time.RFC3339Nano), entry.UpdatedAt.Format(time.RFC3339Nano), pinned)
	if err != nil {
		return fmt.Errorf("memory: sqlite write: %w", err)
	}
	return nil
}

// Search returns entries matching query via FTS5. An empty query returns every
// entry ordered by most-recently-updated. A non-empty query is tokenised into
// prefix terms ANDed together (matching the FlatFileProvider's intuitive
// substring feel while letting SQLite do the heavy lifting).
func (p *SQLiteProvider) Search(ctx context.Context, query string) ([]Entry, error) {
	if err := p.ensureOpen(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if query == "" {
		return p.queryAll(ctx)
	}
	match := buildFTSMatch(query)
	if match == "" {
		return nil, nil
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT e.id, e.kind, e.title, e.body, e.tags, e.created_at, e.updated_at, e.pinned
		FROM entries e
		JOIN entries_fts f ON f.rowid = e.rowid
		WHERE entries_fts MATCH ?
		ORDER BY e.updated_at DESC
	`, match)
	if err != nil {
		return nil, fmt.Errorf("memory: sqlite search: %w", err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

// Delete removes the entry by ID. Idempotent.
func (p *SQLiteProvider) Delete(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if err := p.ensureOpen(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.db.ExecContext(ctx, `DELETE FROM entries WHERE id = ?`, id); err != nil {
		return fmt.Errorf("memory: sqlite delete: %w", err)
	}
	return nil
}

// Prune removes every entry matched by query, except those with Pinned=true.
// An empty query prunes all non-pinned entries. Returns the IDs removed.
func (p *SQLiteProvider) Prune(ctx context.Context, query string) ([]string, error) {
	matches, err := p.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("memory: sqlite prune begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `DELETE FROM entries WHERE id = ? AND pinned = 0`)
	if err != nil {
		return nil, fmt.Errorf("memory: sqlite prune prepare: %w", err)
	}
	defer stmt.Close()

	var removed []string
	for _, e := range matches {
		if e.Pinned {
			continue
		}
		res, err := stmt.ExecContext(ctx, e.ID)
		if err != nil {
			return removed, fmt.Errorf("memory: sqlite prune row: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed = append(removed, e.ID)
		}
	}
	if err := tx.Commit(); err != nil {
		return removed, fmt.Errorf("memory: sqlite prune commit: %w", err)
	}
	return removed, nil
}

// Compress is a no-op for SQLite — entry curation is agent-driven via Write.
// A future implementation could VACUUM here on idle.
func (p *SQLiteProvider) Compress(_ context.Context) error { return nil }

// Shutdown closes the underlying database handle. Safe to call multiple times.
func (p *SQLiteProvider) Shutdown(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db == nil {
		return nil
	}
	err := p.db.Close()
	p.db = nil
	if err != nil {
		return fmt.Errorf("memory: sqlite close: %w", err)
	}
	return nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

var errNotInitialized = errors.New("memory: sqlite provider not initialized; call Initialize first")

func (p *SQLiteProvider) ensureOpen() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db == nil {
		return errNotInitialized
	}
	return nil
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS entries (
			id          TEXT PRIMARY KEY,
			kind        TEXT NOT NULL,
			title       TEXT NOT NULL,
			body        TEXT NOT NULL,
			tags        TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			pinned      INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS entries_updated_idx ON entries(updated_at DESC)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
			title, body, tags,
			content='entries', content_rowid='rowid',
			tokenize='unicode61 remove_diacritics 2'
		)`,
		`CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON entries BEGIN
			INSERT INTO entries_fts(rowid, title, body, tags)
			VALUES (new.rowid, new.title, new.body, new.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON entries BEGIN
			INSERT INTO entries_fts(entries_fts, rowid, title, body, tags)
			VALUES ('delete', old.rowid, old.title, old.body, old.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS entries_au AFTER UPDATE ON entries BEGIN
			INSERT INTO entries_fts(entries_fts, rowid, title, body, tags)
			VALUES ('delete', old.rowid, old.title, old.body, old.tags);
			INSERT INTO entries_fts(rowid, title, body, tags)
			VALUES (new.rowid, new.title, new.body, new.tags);
		END`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("memory: sqlite schema (%s): %w", firstLine(s), err)
		}
	}
	return nil
}

// buildFTSMatch sanitises a free-form query into an FTS5 MATCH expression. We
// strip non-alphanumerics, lowercase, and append a prefix glob to each token —
// so "rotate keys" becomes `"rotate"* AND "keys"*`. Returns "" if the query
// produces no usable tokens (caller should treat that as no match).
func buildFTSMatch(q string) string {
	var tokens []string
	var cur strings.Builder
	for _, r := range strings.ToLower(q) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			cur.WriteRune(r)
		default:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		parts[i] = `"` + t + `"*`
	}
	return strings.Join(parts, " AND ")
}

func (p *SQLiteProvider) queryAll(ctx context.Context) ([]Entry, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, kind, title, body, tags, created_at, updated_at, pinned
		FROM entries
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("memory: sqlite list: %w", err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var (
			e                  Entry
			kind, tags         string
			createdAt, updated string
			pinned             int
		)
		if err := rows.Scan(&e.ID, &kind, &e.Title, &e.Body, &tags, &createdAt, &updated, &pinned); err != nil {
			return nil, fmt.Errorf("memory: sqlite scan: %w", err)
		}
		e.Kind = Kind(kind)
		e.Tags = splitTags(tags)
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		e.Pinned = pinned != 0
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: sqlite rows: %w", err)
	}
	return out, nil
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// Compile-time check that SQLiteProvider satisfies the Provider interface.
var _ Provider = (*SQLiteProvider)(nil)

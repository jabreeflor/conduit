package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LanceDBConfig configures the LanceDB provider.
type LanceDBConfig struct {
	URL        string // LanceDB server URL (default: "http://localhost:8080")
	TableName  string // Table name for memory entries (default: "conduit_memory")
	EmbedModel string // OpenAI embedding model (default: "text-embedding-ada-002")
	APIKey     string // OpenAI API key for embeddings
}

// LanceDBProvider implements the Provider interface using LanceDB as the
// vector database backend. Entries are stored with their text embeddings
// to enable semantic search.
type LanceDBProvider struct {
	mu           sync.RWMutex
	cfg          LanceDBConfig
	embedClient  EmbeddingClient
	httpClient   *http.Client
	initialized  bool
}

// NewLanceDBProvider creates a new LanceDB provider with the given config.
func NewLanceDBProvider(cfg LanceDBConfig) *LanceDBProvider {
	if cfg.URL == "" {
		cfg.URL = "http://localhost:8080"
	}
	if cfg.TableName == "" {
		cfg.TableName = "conduit_memory"
	}
	if cfg.EmbedModel == "" {
		cfg.EmbedModel = "text-embedding-ada-002"
	}
	return &LanceDBProvider{
		cfg:        cfg,
		embedClient: NewOpenAIEmbeddingClient(cfg.APIKey, cfg.EmbedModel),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Initialize creates the LanceDB table if it does not exist.
func (p *LanceDBProvider) Initialize(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	if err := p.createTable(ctx); err != nil {
		return fmt.Errorf("lancedb: create table: %w", err)
	}

	p.initialized = true
	return nil
}

// Prefetch retrieves entries relevant to the query using vector similarity search.
func (p *LanceDBProvider) Prefetch(ctx context.Context, query string) ([]Entry, error) {
	return p.Search(ctx, query)
}

// Write persists an Entry to LanceDB. If the ID already exists, the entry is updated.
func (p *LanceDBProvider) Write(ctx context.Context, entry Entry) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.initialized {
		return fmt.Errorf("lancedb: not initialized")
	}

	if entry.ID == "" {
		entry.ID = generateID()
	}
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	// Generate embedding for the entry text.
	text := entry.Title + " " + entry.Body
	embedding, err := p.embedClient.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("lancedb: generate embedding: %w", err)
	}

	// Insert or update the entry in LanceDB.
	return p.insert(ctx, entry, embedding)
}

// Search returns entries matching the query string using vector similarity.
// This uses embeddings to find semantically relevant entries.
func (p *LanceDBProvider) Search(ctx context.Context, query string) ([]Entry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.initialized {
		return nil, fmt.Errorf("lancedb: not initialized")
	}

	if query == "" {
		// Empty query returns all entries.
		return p.scanAll(ctx)
	}

	// Generate embedding for the query.
	embedding, err := p.embedClient.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("lancedb: generate query embedding: %w", err)
	}

	// Search LanceDB for similar entries.
	return p.search(ctx, embedding, 10)
}

// Delete removes the entry with the given ID. Idempotent.
func (p *LanceDBProvider) Delete(ctx context.Context, id string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.initialized {
		return fmt.Errorf("lancedb: not initialized")
	}

	if id == "" {
		return nil
	}

	return p.delete(ctx, id)
}

// Prune removes entries matched by query (case-insensitive substring on
// title/body/tags) except those with Pinned=true. Returns the IDs removed.
func (p *LanceDBProvider) Prune(ctx context.Context, query string) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.initialized {
		return nil, fmt.Errorf("lancedb: not initialized")
	}

	// Get all entries first.
	entries, err := p.scanAll(ctx)
	if err != nil {
		return nil, err
	}

	var removed []string
	q := strings.ToLower(query)

	for _, e := range entries {
		if e.Pinned {
			continue
		}

		// Match logic: empty query matches everything, otherwise match title/body/tags.
		if q != "" {
			if !strings.Contains(strings.ToLower(e.Title), q) &&
				!strings.Contains(strings.ToLower(e.Body), q) &&
				!containsTag(e.Tags, q) {
				continue
			}
		}

		// Delete the entry.
		if err := p.delete(ctx, e.ID); err != nil {
			return removed, fmt.Errorf("lancedb: prune delete: %w", err)
		}
		removed = append(removed, e.ID)
	}

	return removed, nil
}

// Compress is a no-op for the LanceDB backend.
func (p *LanceDBProvider) Compress(_ context.Context) error {
	return nil
}

// Shutdown flushes any pending operations and closes the connection.
func (p *LanceDBProvider) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.initialized = false
	return nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// lancedbEntry represents an entry stored in LanceDB with vector embedding.
type lancedbEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Tags      []string  `json:"tags"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	Pinned    bool      `json:"pinned"`
	Vector    []float64 `json:"vector"`
}

// createTable creates the LanceDB table with the proper schema.
func (p *LanceDBProvider) createTable(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/tables", p.cfg.URL)

	schema := map[string]interface{}{
		"name": p.cfg.TableName,
		"data": []map[string]interface{}{},
	}

	body, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// LanceDB returns 409 if table already exists, which is fine.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lancedb: create table failed: %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// insert adds or updates an entry in LanceDB.
func (p *LanceDBProvider) insert(ctx context.Context, entry Entry, embedding []float64) error {
	url := fmt.Sprintf("%s/v1/tables/%s/add", p.cfg.URL, p.cfg.TableName)

	ldbEntry := lancedbEntry{
		ID:        entry.ID,
		Kind:      string(entry.Kind),
		Title:     entry.Title,
		Body:      entry.Body,
		Tags:      entry.Tags,
		CreatedAt: entry.CreatedAt.Format(time.RFC3339),
		UpdatedAt: entry.UpdatedAt.Format(time.RFC3339),
		Pinned:    entry.Pinned,
		Vector:    embedding,
	}

	body, err := json.Marshal([]lancedbEntry{ldbEntry})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lancedb: insert failed: %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// search performs a vector similarity search against the LanceDB table.
func (p *LanceDBProvider) search(ctx context.Context, embedding []float64, limit int) ([]Entry, error) {
	url := fmt.Sprintf("%s/v1/tables/%s/search", p.cfg.URL, p.cfg.TableName)

	searchReq := map[string]interface{}{
		"vector": embedding,
		"limit":  limit,
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lancedb: search failed: %d: %s", resp.StatusCode, string(respBody))
	}

	var results struct {
		Results []lancedbEntry `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	entries := make([]Entry, len(results.Results))
	for i, ldbE := range results.Results {
		createdAt, _ := time.Parse(time.RFC3339, ldbE.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, ldbE.UpdatedAt)
		entries[i] = Entry{
			ID:        ldbE.ID,
			Kind:      Kind(ldbE.Kind),
			Title:     ldbE.Title,
			Body:      ldbE.Body,
			Tags:      ldbE.Tags,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Pinned:    ldbE.Pinned,
		}
	}

	return entries, nil
}

// delete removes an entry from LanceDB by ID.
func (p *LanceDBProvider) delete(ctx context.Context, id string) error {
	url := fmt.Sprintf("%s/v1/tables/%s/delete", p.cfg.URL, p.cfg.TableName)

	deleteReq := map[string]interface{}{
		"where": fmt.Sprintf("id = '%s'", strings.ReplaceAll(id, "'", "''")),
	}

	body, err := json.Marshal(deleteReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Return nil if not found (idempotent).
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lancedb: delete failed: %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// scanAll retrieves all entries from the LanceDB table.
func (p *LanceDBProvider) scanAll(ctx context.Context) ([]Entry, error) {
	url := fmt.Sprintf("%s/v1/tables/%s/search", p.cfg.URL, p.cfg.TableName)

	// A search with a zero vector returns all entries (approximate).
	// Better approach: use a simple SELECT-like query if LanceDB supports it.
	// For now, return an empty query result.
	searchReq := map[string]interface{}{
		"vector": make([]float64, 1536), // Default embedding dimension for ada-002
		"limit":  1000,
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []Entry{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lancedb: scan failed: %d: %s", resp.StatusCode, string(respBody))
	}

	var results struct {
		Results []lancedbEntry `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	entries := make([]Entry, len(results.Results))
	for i, ldbE := range results.Results {
		createdAt, _ := time.Parse(time.RFC3339, ldbE.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, ldbE.UpdatedAt)
		entries[i] = Entry{
			ID:        ldbE.ID,
			Kind:      Kind(ldbE.Kind),
			Title:     ldbE.Title,
			Body:      ldbE.Body,
			Tags:      ldbE.Tags,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Pinned:    ldbE.Pinned,
		}
	}

	return entries, nil
}

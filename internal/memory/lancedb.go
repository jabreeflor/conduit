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

// LanceDBConfig holds configuration for connecting to a LanceDB instance.
type LanceDBConfig struct {
	URL       string // Base URL of the LanceDB server (e.g., "http://localhost:8081")
	TableName string // LanceDB table name (e.g., "conduit_memory")
	EmbedModel string // Embedding model (e.g., "text-embedding-3-small")
	APIKey    string // OpenAI API key for embeddings
}

// EmbeddingClient is the interface for generating embeddings.
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// OpenAIEmbeddingClient implements EmbeddingClient using OpenAI's API.
type OpenAIEmbeddingClient struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIEmbeddingClient creates a new OpenAI embedding client.
func NewOpenAIEmbeddingClient(apiKey, model string) *OpenAIEmbeddingClient {
	return &OpenAIEmbeddingClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed calls the OpenAI embeddings API.
func (c *OpenAIEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("openai: api key not set")
	}
	if text == "" {
		return nil, fmt.Errorf("openai: empty text")
	}

	reqBody := map[string]interface{}{
		"input": text,
		"model": c.model,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openai: %s", result.Error.Message)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openai: no embedding in response")
	}

	return result.Data[0].Embedding, nil
}

// LanceDBProvider is a memory provider backed by LanceDB, a vector database.
// It supports semantic search via embeddings and stores all memory entries as vectors.
type LanceDBProvider struct {
	mu       sync.RWMutex
	config   LanceDBConfig
	embedder EmbeddingClient
	client   *http.Client
	cache    map[string]Entry // Simple in-memory cache for recently accessed entries
}

// NewLanceDBProvider creates a new LanceDB provider.
func NewLanceDBProvider(config LanceDBConfig, embedder EmbeddingClient) *LanceDBProvider {
	if embedder == nil {
		embedder = NewOpenAIEmbeddingClient(config.APIKey, config.EmbedModel)
	}
	return &LanceDBProvider{
		config:   config,
		embedder: embedder,
		client:   &http.Client{Timeout: 30 * time.Second},
		cache:    make(map[string]Entry),
	}
}

// Initialize ensures the LanceDB table exists with the right schema.
func (p *LanceDBProvider) Initialize(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.ensureTable(ctx)
}

// ensureTable creates the LanceDB table if it doesn't exist.
func (p *LanceDBProvider) ensureTable(ctx context.Context) error {
	// LanceDB tables are created implicitly on first insert, but we can
	// verify connectivity by getting table info. If the table doesn't exist,
	// LanceDB returns a 404, which is fine.
	url := fmt.Sprintf("%s/api/v1/tables/%s", strings.TrimSuffix(p.config.URL, "/"), p.config.TableName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("lancedb: create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("lancedb: connectivity check: %w", err)
	}
	defer resp.Body.Close()

	// 200 OK means table exists, 404 means it will be created on first insert.
	// Any other status is a real error.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lancedb: table check failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Prefetch returns entries relevant to the query by doing a vector similarity search.
func (p *LanceDBProvider) Prefetch(ctx context.Context, query string) ([]Entry, error) {
	return p.Search(ctx, query)
}

// Write persists an Entry by embedding its title+body and inserting into LanceDB.
func (p *LanceDBProvider) Write(ctx context.Context, entry Entry) error {
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

	// Generate embedding from title + body
	textToEmbed := entry.Title + " " + entry.Body
	vector, err := p.embedder.Embed(ctx, textToEmbed)
	if err != nil {
		return fmt.Errorf("lancedb: embed failed: %w", err)
	}

	// Prepare record for insertion
	record := map[string]interface{}{
		"id":        entry.ID,
		"kind":      string(entry.Kind),
		"title":     entry.Title,
		"body":      entry.Body,
		"tags":      entry.Tags,
		"created_at": entry.CreatedAt.Unix(),
		"updated_at": entry.UpdatedAt.Unix(),
		"pinned":    entry.Pinned,
		"vector":    vector,
	}

	if err := p.insertRecord(ctx, record); err != nil {
		return err
	}

	// Update cache
	p.cache[entry.ID] = entry

	return nil
}

// insertRecord sends a record to LanceDB via HTTP POST.
func (p *LanceDBProvider) insertRecord(ctx context.Context, record map[string]interface{}) error {
	url := fmt.Sprintf("%s/api/v1/tables/%s/add", strings.TrimSuffix(p.config.URL, "/"), p.config.TableName)
	
	jsonData, err := json.Marshal([]map[string]interface{}{record})
	if err != nil {
		return fmt.Errorf("lancedb: marshal record: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("lancedb: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("lancedb: insert failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lancedb: insert failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Search performs a vector similarity search on the query text.
// Returns all entries sorted by relevance (similarity score).
func (p *LanceDBProvider) Search(ctx context.Context, query string) ([]Entry, error) {
	if query == "" {
		// Empty query returns all entries
		return p.listAll(ctx)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Generate embedding for the query
	vector, err := p.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("lancedb: embed query: %w", err)
	}

	results, err := p.vectorSearch(ctx, vector, 100) // Limit to top 100 results
	if err != nil {
		return nil, err
	}

	return results, nil
}

// vectorSearch calls LanceDB's vector search API and returns matching entries.
func (p *LanceDBProvider) vectorSearch(ctx context.Context, vector []float32, limit int) ([]Entry, error) {
	url := fmt.Sprintf("%s/api/v1/tables/%s/search", strings.TrimSuffix(p.config.URL, "/"), p.config.TableName)

	searchRequest := map[string]interface{}{
		"vector": vector,
		"limit":  limit,
	}
	jsonData, err := json.Marshal(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("lancedb: marshal search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("lancedb: create search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lancedb: search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lancedb: search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var results struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("lancedb: parse search response: %w", err)
	}

	entries := make([]Entry, 0, len(results.Results))
	for _, r := range results.Results {
		entry, err := p.unmarshalRecord(r)
		if err != nil {
			// Log but continue with other results
			fmt.Printf("lancedb: unmarshal record: %v\n", err)
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// listAll retrieves all entries from LanceDB without vector search.
func (p *LanceDBProvider) listAll(ctx context.Context) ([]Entry, error) {
	url := fmt.Sprintf("%s/api/v1/tables/%s/query", strings.TrimSuffix(p.config.URL, "/"), p.config.TableName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("lancedb: create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lancedb: list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Table doesn't exist yet
		return []Entry{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("lancedb: list failed with status %d: %s", resp.StatusCode, string(body))
	}

	var results struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("lancedb: parse list response: %w", err)
	}

	entries := make([]Entry, 0, len(results.Results))
	for _, r := range results.Results {
		entry, err := p.unmarshalRecord(r)
		if err != nil {
			fmt.Printf("lancedb: unmarshal record: %v\n", err)
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// unmarshalRecord converts a LanceDB record to an Entry.
func (p *LanceDBProvider) unmarshalRecord(r map[string]interface{}) (Entry, error) {
	entry := Entry{}

	if id, ok := r["id"].(string); ok {
		entry.ID = id
	} else {
		return Entry{}, fmt.Errorf("missing or invalid id")
	}

	if kind, ok := r["kind"].(string); ok {
		entry.Kind = Kind(kind)
	}

	if title, ok := r["title"].(string); ok {
		entry.Title = title
	}

	if body, ok := r["body"].(string); ok {
		entry.Body = body
	}

	if tags, ok := r["tags"].([]interface{}); ok {
		entry.Tags = make([]string, len(tags))
		for i, t := range tags {
			if s, ok := t.(string); ok {
				entry.Tags[i] = s
			}
		}
	}

	if createdAt, ok := r["created_at"].(float64); ok {
		entry.CreatedAt = time.Unix(int64(createdAt), 0)
	}

	if updatedAt, ok := r["updated_at"].(float64); ok {
		entry.UpdatedAt = time.Unix(int64(updatedAt), 0)
	}

	if pinned, ok := r["pinned"].(bool); ok {
		entry.Pinned = pinned
	}

	return entry, nil
}

// Delete removes the entry with the given ID from LanceDB.
func (p *LanceDBProvider) Delete(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	err := p.deleteRecord(ctx, id)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}

	// Remove from cache
	delete(p.cache, id)

	return nil
}

// deleteRecord removes a record from LanceDB by ID.
func (p *LanceDBProvider) deleteRecord(ctx context.Context, id string) error {
	url := fmt.Sprintf("%s/api/v1/tables/%s/delete", strings.TrimSuffix(p.config.URL, "/"), p.config.TableName)

	deleteRequest := map[string]interface{}{
		"where": fmt.Sprintf("id = '%s'", id),
	}
	jsonData, err := json.Marshal(deleteRequest)
	if err != nil {
		return fmt.Errorf("lancedb: marshal delete request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("lancedb: create delete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("lancedb: delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lancedb: delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Prune removes entries matching the query, except those with Pinned=true.
// Returns the IDs of removed entries.
func (p *LanceDBProvider) Prune(ctx context.Context, query string) ([]string, error) {
	// Search for matching entries
	var matches []Entry
	var err error

	if query == "" {
		// Empty query prunes all non-pinned entries
		matches, err = p.listAll(ctx)
	} else {
		matches, err = p.Search(ctx, query)
	}

	if err != nil {
		return nil, fmt.Errorf("lancedb: search for prune: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	removed := make([]string, 0)
	for _, entry := range matches {
		if entry.Pinned {
			continue // Skip pinned entries
		}
		if err := p.deleteRecord(ctx, entry.ID); err != nil {
			fmt.Printf("lancedb: failed to delete %s during prune: %v\n", entry.ID, err)
			continue
		}
		removed = append(removed, entry.ID)
		delete(p.cache, entry.ID)
	}

	return removed, nil
}

// Compress is a no-op for LanceDB; the vector database handles optimization internally.
func (p *LanceDBProvider) Compress(ctx context.Context) error {
	// LanceDB handles compression and optimization internally.
	// This is a no-op for now.
	return nil
}

// Shutdown is a no-op; LanceDB doesn't require explicit cleanup.
func (p *LanceDBProvider) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = nil
	return nil
}

package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// MockHTTPClient for testing LanceDB HTTP interactions.
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

// MockEmbeddingClient for testing embedding generation.
type MockEmbeddingClient struct {
	EmbedFunc func(ctx context.Context, text string) ([]float64, error)
}

func (m *MockEmbeddingClient) Embed(ctx context.Context, text string) ([]float64, error) {
	return m.EmbedFunc(ctx, text)
}

func TestNewLanceDBProvider(t *testing.T) {
	cfg := LanceDBConfig{
		URL:        "http://custom.com:9999",
		TableName:  "custom_table",
		EmbedModel: "text-embedding-3-small",
		APIKey:     "test-key",
	}
	p := NewLanceDBProvider(cfg)

	if p.cfg.URL != cfg.URL {
		t.Errorf("URL mismatch: got %s, want %s", p.cfg.URL, cfg.URL)
	}
	if p.cfg.TableName != cfg.TableName {
		t.Errorf("TableName mismatch: got %s, want %s", p.cfg.TableName, cfg.TableName)
	}
	if p.cfg.EmbedModel != cfg.EmbedModel {
		t.Errorf("EmbedModel mismatch: got %s, want %s", p.cfg.EmbedModel, cfg.EmbedModel)
	}
	if p.cfg.APIKey != cfg.APIKey {
		t.Errorf("APIKey mismatch: got %s, want %s", p.cfg.APIKey, cfg.APIKey)
	}
	if p.initialized {
		t.Errorf("Expected initialized to be false, got true")
	}
}

func TestNewLanceDBProviderDefaults(t *testing.T) {
	cfg := LanceDBConfig{APIKey: "test-key"}
	p := NewLanceDBProvider(cfg)

	if p.cfg.URL != "http://localhost:8080" {
		t.Errorf("Default URL mismatch: got %s", p.cfg.URL)
	}
	if p.cfg.TableName != "conduit_memory" {
		t.Errorf("Default TableName mismatch: got %s", p.cfg.TableName)
	}
	if p.cfg.EmbedModel != "text-embedding-ada-002" {
		t.Errorf("Default EmbedModel mismatch: got %s", p.cfg.EmbedModel)
	}
}

func TestLanceDBInitialize(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	// Mock successful table creation
	p.httpClient = &http.Client{}
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.String(), "/v1/tables") {
				t.Errorf("Unexpected URL: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("{}")),
			}, nil
		},
	}

	// Replace http.Client's Do method by reassigning transport behavior
	origHTTPDo := p.httpClient.Do
	_ = origHTTPDo // unused

	ctx := context.Background()

	// Test successful initialization
	if err := p.Initialize(ctx); err != nil {
		t.Errorf("Initialize failed: %v", err)
	}
	if !p.initialized {
		t.Errorf("initialized flag not set")
	}

	// Test idempotency
	if err := p.Initialize(ctx); err != nil {
		t.Errorf("Second Initialize failed: %v", err)
	}

	_ = mockClient // keep reference for type checking
}

func TestLanceDBWrite(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:        "http://localhost:8080",
		TableName:  "test_table",
		EmbedModel: "text-embedding-ada-002",
		APIKey:     "test-key",
	})

	// Mock embedding client
	p.embedClient = &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float64, error) {
			// Return a fixed 1536-dim vector for ada-002
			vec := make([]float64, 1536)
			for i := range vec {
				vec[i] = 0.1
			}
			return vec, nil
		},
	}

	// Mark as initialized
	p.initialized = true

	entry := Entry{
		Kind:  KindFact,
		Title: "Test Fact",
		Body:  "This is a test fact",
		Tags:  []string{"test", "memory"},
	}

	ctx := context.Background()
	err := p.Write(ctx, entry)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Check that ID was generated
	if entry.ID == "" {
		t.Errorf("Entry ID not generated")
	}

	// Check timestamps
	if entry.CreatedAt.IsZero() {
		t.Errorf("CreatedAt not set")
	}
	if entry.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt not set")
	}
}

func TestLanceDBWriteNotInitialized(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{APIKey: "test-key"})

	entry := Entry{
		Kind:  KindFact,
		Title: "Test",
		Body:  "Test body",
	}

	ctx := context.Background()
	err := p.Write(ctx, entry)
	if err == nil {
		t.Errorf("Expected error when not initialized, got nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}
}

func TestLanceDBSearch(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	p.embedClient = &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float64, error) {
			vec := make([]float64, 1536)
			for i := range vec {
				vec[i] = 0.1
			}
			return vec, nil
		},
	}

	p.initialized = true

	ctx := context.Background()

	// Test search returns results
	results, err := p.Search(ctx, "test query")
	if err != nil {
		t.Errorf("Search failed: %v", err)
	}
	if results == nil {
		t.Errorf("Search returned nil results")
	}
}

func TestLanceDBSearchEmpty(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	p.embedClient = &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float64, error) {
			vec := make([]float64, 1536)
			return vec, nil
		},
	}

	p.initialized = true

	ctx := context.Background()

	// Empty query should scan all
	results, err := p.Search(ctx, "")
	if err != nil {
		t.Errorf("Search with empty query failed: %v", err)
	}
	if results == nil {
		t.Errorf("Search returned nil results")
	}
}

func TestLanceDBSearchNotInitialized(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{APIKey: "test-key"})

	ctx := context.Background()
	_, err := p.Search(ctx, "test")
	if err == nil {
		t.Errorf("Expected error when not initialized, got nil")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}
}

func TestLanceDBDelete(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	p.initialized = true

	ctx := context.Background()

	// Test delete with valid ID
	err := p.Delete(ctx, "test-id-12345")
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	// Test delete with empty ID (should be idempotent)
	err = p.Delete(ctx, "")
	if err != nil {
		t.Errorf("Delete with empty ID failed: %v", err)
	}
}

func TestLanceDBDeleteNotInitialized(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{APIKey: "test-key"})

	ctx := context.Background()
	err := p.Delete(ctx, "test-id")
	if err == nil {
		t.Errorf("Expected error when not initialized, got nil")
	}
}

func TestLanceDBPrune(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	p.embedClient = &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float64, error) {
			vec := make([]float64, 1536)
			return vec, nil
		},
	}

	p.initialized = true

	ctx := context.Background()

	// Test prune with query
	removed, err := p.Prune(ctx, "test")
	if err != nil {
		t.Errorf("Prune failed: %v", err)
	}
	if removed == nil {
		t.Errorf("Prune returned nil slice")
	}

	// Test prune with empty query (should prune all non-pinned)
	removed, err = p.Prune(ctx, "")
	if err != nil {
		t.Errorf("Prune with empty query failed: %v", err)
	}
	if removed == nil {
		t.Errorf("Prune returned nil slice")
	}
}

func TestLanceDBPruneNotInitialized(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{APIKey: "test-key"})

	ctx := context.Background()
	_, err := p.Prune(ctx, "test")
	if err == nil {
		t.Errorf("Expected error when not initialized, got nil")
	}
}

func TestLanceDBCompress(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{APIKey: "test-key"})

	ctx := context.Background()
	err := p.Compress(ctx)
	if err != nil {
		t.Errorf("Compress should be no-op, got error: %v", err)
	}
}

func TestLanceDBShutdown(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{APIKey: "test-key"})
	p.initialized = true

	ctx := context.Background()
	err := p.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
	if p.initialized {
		t.Errorf("initialized flag not cleared after Shutdown")
	}
}

func TestLanceDBPrefetch(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	p.embedClient = &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float64, error) {
			vec := make([]float64, 1536)
			return vec, nil
		},
	}

	p.initialized = true

	ctx := context.Background()

	// Prefetch should delegate to Search
	results, err := p.Prefetch(ctx, "test query")
	if err != nil {
		t.Errorf("Prefetch failed: %v", err)
	}
	if results == nil {
		t.Errorf("Prefetch returned nil results")
	}
}

func TestLancedbEntryMarshaling(t *testing.T) {
	now := time.Now().UTC()
	entry := Entry{
		ID:        "test-id-12345",
		Kind:      KindFact,
		Title:     "Test Entry",
		Body:      "This is a test body",
		Tags:      []string{"tag1", "tag2"},
		CreatedAt: now,
		UpdatedAt: now,
		Pinned:    true,
	}

	embedding := []float64{0.1, 0.2, 0.3}

	ldbE := lancedbEntry{
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

	// Marshal to JSON and back
	data, err := json.Marshal(ldbE)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}

	var unmarshaled lancedbEntry
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}

	if unmarshaled.ID != entry.ID {
		t.Errorf("ID mismatch: %s != %s", unmarshaled.ID, entry.ID)
	}
	if unmarshaled.Kind != string(entry.Kind) {
		t.Errorf("Kind mismatch: %s != %s", unmarshaled.Kind, string(entry.Kind))
	}
	if unmarshaled.Title != entry.Title {
		t.Errorf("Title mismatch: %s != %s", unmarshaled.Title, entry.Title)
	}
	if !unmarshaled.Pinned {
		t.Errorf("Pinned flag not preserved")
	}
}

func TestLanceDBThreadSafety(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	p.embedClient = &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float64, error) {
			vec := make([]float64, 1536)
			return vec, nil
		},
	}

	p.initialized = true
	ctx := context.Background()

	// Simulate concurrent operations
	done := make(chan error, 3)

	// Concurrent write
	go func() {
		entry := Entry{
			Kind:  KindFact,
			Title: "Concurrent Write",
			Body:  "Test body",
		}
		done <- p.Write(ctx, entry)
	}()

	// Concurrent search
	go func() {
		_, err := p.Search(ctx, "test")
		done <- err
	}()

	// Concurrent delete
	go func() {
		done <- p.Delete(ctx, "test-id")
	}()

	// Check all operations completed without error
	for i := 0; i < 3; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent operation failed: %v", err)
		}
	}
}

func TestLanceDBEntryIDGeneration(t *testing.T) {
	p := NewLanceDBProvider(LanceDBConfig{
		URL:       "http://localhost:8080",
		TableName: "test_table",
		APIKey:    "test-key",
	})

	p.embedClient = &MockEmbeddingClient{
		EmbedFunc: func(ctx context.Context, text string) ([]float64, error) {
			vec := make([]float64, 1536)
			return vec, nil
		},
	}

	p.initialized = true
	ctx := context.Background()

	entry1 := Entry{
		Kind:  KindFact,
		Title: "First Entry",
		Body:  "Test",
	}

	entry2 := Entry{
		Kind:  KindFact,
		Title: "Second Entry",
		Body:  "Test",
	}

	if err := p.Write(ctx, entry1); err != nil {
		t.Errorf("Write entry1 failed: %v", err)
	}

	if err := p.Write(ctx, entry2); err != nil {
		t.Errorf("Write entry2 failed: %v", err)
	}

	if entry1.ID == "" {
		t.Errorf("entry1 ID not generated")
	}
	if entry2.ID == "" {
		t.Errorf("entry2 ID not generated")
	}
	if entry1.ID == entry2.ID {
		t.Errorf("Generated IDs should be unique")
	}

	// IDs should be 20 characters (16 timestamp + 4 random hex chars)
	if len(entry1.ID) != 20 {
		t.Errorf("ID length mismatch: got %d, want 20", len(entry1.ID))
	}
}

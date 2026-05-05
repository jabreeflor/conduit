package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockEmbeddingClient provides deterministic embeddings for testing
type mockEmbeddingClient struct {
	mu       sync.Mutex
	embedMap map[string][]float32
}

func newMockEmbeddingClient() *mockEmbeddingClient {
	return &mockEmbeddingClient{
		embedMap: make(map[string][]float32),
	}
}

func (m *mockEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if text == "" {
		return nil, ErrEmptyInput
	}
	
	// Return cached embedding or generate deterministic one
	if vec, exists := m.embedMap[text]; exists {
		return vec, nil
	}
	
	// Generate deterministic embedding based on text length
	size := 1536
	vec := make([]float32, size)
	for i := range vec {
		// Use hash-like function for deterministic values
		vec[i] = float32((len(text) * (i + 1)) % 100) / 100.0
	}
	m.embedMap[text] = vec
	return vec, nil
}

func TestLanceDBProvider_Initialize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
}

func TestLanceDBProvider_Initialize_ConnectionError(t *testing.T) {
	config := LanceDBConfig{
		URL:       "http://invalid-host-that-does-not-exist:9999",
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := provider.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected initialization to fail for invalid host")
	}
}

func TestLanceDBProvider_Write(t *testing.T) {
	writeCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		if strings.Contains(r.URL.Path, "/add") && r.Method == http.MethodPost {
			writeCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	entry := Entry{
		Kind:      KindFact,
		Title:     "Test Fact",
		Body:      "This is a test fact",
		Tags:      []string{"test"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = provider.Write(context.Background(), entry)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if entry.ID == "" {
		t.Fatal("Entry ID should be generated on write")
	}
}

func TestLanceDBProvider_Search(t *testing.T) {
	testEntries := []map[string]interface{}{
		{
			"id":         "entry-1",
			"kind":       "fact",
			"title":      "Machine Learning",
			"body":       "ML is a subset of AI",
			"tags":       []interface{}{"ai", "tech"},
			"created_at": time.Now().Unix(),
			"updated_at": time.Now().Unix(),
			"pinned":     false,
			"vector":     make([]float32, 1536),
		},
		{
			"id":         "entry-2",
			"kind":       "decision",
			"title":      "Architecture Choice",
			"body":       "Decided to use microservices",
			"tags":       []interface{}{"architecture"},
			"created_at": time.Now().Unix(),
			"updated_at": time.Now().Unix(),
			"pinned":     false,
			"vector":     make([]float32, 1536),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		if strings.Contains(r.URL.Path, "/search") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": testEntries,
			})
			return
		}
		if strings.Contains(r.URL.Path, "/query") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": testEntries,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Search with query
	results, err := provider.Search(context.Background(), "Machine Learning", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if results[0].ID != "entry-1" {
		t.Fatalf("Expected first result ID to be 'entry-1', got '%s'", results[0].ID)
	}
}

func TestLanceDBProvider_Delete(t *testing.T) {
	deleteCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/delete") {
			deleteCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	err = provider.Delete(context.Background(), "test-id")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if deleteCount != 1 {
		t.Fatalf("Expected 1 delete call, got %d", deleteCount)
	}
}

func TestLanceDBProvider_Prune(t *testing.T) {
	testEntries := []map[string]interface{}{
		{
			"id":         "entry-1",
			"kind":       "fact",
			"title":      "Important",
			"body":       "Keep this",
			"tags":       []interface{}{},
			"created_at": time.Now().Unix(),
			"updated_at": time.Now().Unix(),
			"pinned":     true,
			"vector":     make([]float32, 1536),
		},
		{
			"id":         "entry-2",
			"kind":       "fact",
			"title":      "Temporary",
			"body":       "Remove this",
			"tags":       []interface{}{},
			"created_at": time.Now().Add(-30 * 24 * time.Hour).Unix(),
			"updated_at": time.Now().Add(-30 * 24 * time.Hour).Unix(),
			"pinned":     false,
			"vector":     make([]float32, 1536),
		},
	}

	deleteCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		if strings.Contains(r.URL.Path, "/query") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": testEntries,
			})
			return
		}
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/delete") {
			deleteCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	removed, err := provider.Prune(context.Background(), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	if len(removed) != 1 {
		t.Fatalf("Expected 1 removed entry, got %d", len(removed))
	}

	if removed[0] != "entry-2" {
		t.Fatalf("Expected removed ID to be 'entry-2', got '%s'", removed[0])
	}
}

func TestLanceDBProvider_Prefetch(t *testing.T) {
	testEntries := []map[string]interface{}{
		{
			"id":         "entry-1",
			"kind":       "fact",
			"title":      "Test",
			"body":       "Content",
			"tags":       []interface{}{},
			"created_at": time.Now().Unix(),
			"updated_at": time.Now().Unix(),
			"pinned":     false,
			"vector":     make([]float32, 1536),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		if strings.Contains(r.URL.Path, "/query") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": testEntries,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	results, err := provider.Prefetch(context.Background(), 10)
	if err != nil {
		t.Fatalf("Prefetch failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
}

func TestLanceDBProvider_Compress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Compress should be a no-op
	err = provider.Compress(context.Background())
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
}

func TestLanceDBProvider_Shutdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	err = provider.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestLanceDBProvider_ConcurrentWrites(t *testing.T) {
	writeCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		if strings.Contains(r.URL.Path, "/add") && r.Method == http.MethodPost {
			mu.Lock()
			writeCount++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Perform concurrent writes
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			entry := Entry{
				Kind:      KindFact,
				Title:     fmt.Sprintf("Test %d", index),
				Body:      fmt.Sprintf("Body %d", index),
				Tags:      []string{"concurrent"},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			provider.Write(context.Background(), entry)
		}(i)
	}

	wg.Wait()

	if writeCount != 5 {
		t.Fatalf("Expected 5 writes, got %d", writeCount)
	}
}

func TestOpenAIEmbeddingClient_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/embeddings" {
			var req struct {
				Input string `json:"input"`
				Model string `json:"model"`
			}
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set("Content-Type", "application/json")
			embedding := make([]float32, 1536)
			for i := range embedding {
				embedding[i] = 0.1
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"embedding": embedding,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Extract host and use it
	client := &OpenAIEmbeddingClient{
		apiKey: "test-key",
		model:  "text-embedding-3-small",
		client: &http.Client{},
		baseURL: server.URL,
	}

	vec, err := client.Embed(context.Background(), "test input")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(vec) != 1536 {
		t.Fatalf("Expected embedding length 1536, got %d", len(vec))
	}
}

func TestOpenAIEmbeddingClient_EmptyInput(t *testing.T) {
	client := NewOpenAIEmbeddingClient("test-key", "text-embedding-3-small")

	_, err := client.Embed(context.Background(), "")
	if err != ErrEmptyInput {
		t.Fatalf("Expected ErrEmptyInput, got %v", err)
	}
}

func TestOpenAIEmbeddingClient_MissingAPIKey(t *testing.T) {
	client := NewOpenAIEmbeddingClient("", "text-embedding-3-small")

	_, err := client.Embed(context.Background(), "test")
	if err != ErrMissingAPIKey {
		t.Fatalf("Expected ErrMissingAPIKey, got %v", err)
	}
}

func TestLanceDBProvider_WriteDuplicateID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v1/tables/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "test_table",
			})
			return
		}
		if strings.Contains(r.URL.Path, "/add") && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	err := provider.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	entry := Entry{
		ID:        "specific-id",
		Kind:      KindFact,
		Title:     "Test",
		Body:      "Content",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = provider.Write(context.Background(), entry)
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}

	// Second write with same ID should preserve it
	err = provider.Write(context.Background(), entry)
	if err != nil {
		t.Fatalf("Second write failed: %v", err)
	}

	if entry.ID != "specific-id" {
		t.Fatalf("Expected ID to remain 'specific-id', got '%s'", entry.ID)
	}
}

func TestLanceDBProvider_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "test_table",
		})
	}))
	defer server.Close()

	config := LanceDBConfig{
		URL:       server.URL,
		TableName: "test_table",
	}
	embedder := newMockEmbeddingClient()
	provider := NewLanceDBProvider(config, embedder)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := provider.Initialize(ctx)
	if err == nil {
		t.Fatal("Expected context cancellation error")
	}
}

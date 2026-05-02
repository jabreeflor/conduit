package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_returnsToolWithCorrectName(t *testing.T) {
	tool := New(DefaultConfig())
	if tool.Name != "web_search" {
		t.Fatalf("Name = %q, want web_search", tool.Name)
	}
}

func TestRun_disabledMode(t *testing.T) {
	tool := New(Config{Mode: ModeDisabled})
	raw, _ := json.Marshal(map[string]string{"url": "http://example.com"})
	result, err := tool.Run(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for disabled mode")
	}
	if result.Text != "web search is disabled" {
		t.Fatalf("Text = %q, want disabled message", result.Text)
	}
}

func TestRun_blocksPostMethod(t *testing.T) {
	tool := New(Config{Mode: ModeLive})
	raw, _ := json.Marshal(map[string]string{"url": "http://example.com", "method": "POST"})
	result, err := tool.Run(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for blocked method")
	}
}

func TestRun_blocksDeleteMethod(t *testing.T) {
	tool := New(Config{Mode: ModeLive})
	raw, _ := json.Marshal(map[string]string{"url": "http://example.com", "method": "DELETE"})
	result, err := tool.Run(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for DELETE method")
	}
}

func TestRun_blocksPutMethod(t *testing.T) {
	tool := New(Config{Mode: ModeLive})
	raw, _ := json.Marshal(map[string]string{"url": "http://example.com", "method": "PUT"})
	result, err := tool.Run(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for PUT method")
	}
}

func TestRun_liveMode_fetchesAndReturnsTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><head><title>Hello World</title></head><body>Some content here</body></html>")
	}))
	defer srv.Close()

	tool := New(Config{Mode: ModeLive})
	raw, _ := json.Marshal(map[string]string{"url": srv.URL})
	result, err := tool.Run(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Text)
	}
	if !strContains(result.Text, "Hello World") {
		t.Fatalf("Text = %q, expected title 'Hello World'", result.Text)
	}
}

func TestRun_cachedMode_storesAndReloadsResult(t *testing.T) {
	cacheDir := t.TempDir()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><head><title>Cached Page</title></head><body>body text</body></html>")
	}))
	defer srv.Close()

	cfg := Config{Mode: ModeCached, CacheDir: cacheDir}
	tool := New(cfg)
	raw, _ := json.Marshal(map[string]string{"url": srv.URL})

	// First call — should hit the server and cache.
	r1, err := tool.Run(context.Background(), raw)
	if err != nil || r1.IsError {
		t.Fatalf("first call failed: err=%v text=%s", err, r1.Text)
	}

	// Second call — should return from cache without hitting the server.
	r2, err := tool.Run(context.Background(), raw)
	if err != nil || r2.IsError {
		t.Fatalf("second call failed: err=%v text=%s", err, r2.Text)
	}
	if callCount != 1 {
		t.Fatalf("server callCount = %d, want 1 (second call should use cache)", callCount)
	}
	if r1.Text != r2.Text {
		t.Fatal("cached result text differs from live result")
	}
}

func TestRun_cachedMode_cacheFileHasExpectedKey(t *testing.T) {
	cacheDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "<html><head><title>T</title></head><body>B</body></html>")
	}))
	defer srv.Close()

	cfg := Config{Mode: ModeCached, CacheDir: cacheDir}
	tool := New(cfg)
	raw, _ := json.Marshal(map[string]string{"url": srv.URL})
	if _, err := tool.Run(context.Background(), raw); err != nil {
		t.Fatal(err)
	}

	key := cacheKey(srv.URL)
	path := filepath.Join(cacheDir, key+cacheFileExt)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not found at %s: %v", path, err)
	}
}

func TestRun_missingURL_returnsError(t *testing.T) {
	tool := New(Config{Mode: ModeLive})
	raw, _ := json.Marshal(map[string]string{})
	result, err := tool.Run(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when url is missing")
	}
}

func TestRun_resultDataContainsURLField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "<html><head><title>T</title></head><body>B</body></html>")
	}))
	defer srv.Close()

	tool := New(Config{Mode: ModeLive})
	raw, _ := json.Marshal(map[string]string{"url": srv.URL})
	result, err := tool.Run(context.Background(), raw)
	if err != nil || result.IsError {
		t.Fatalf("unexpected failure: err=%v text=%s", err, result.Text)
	}
	if result.Data["url"] != srv.URL {
		t.Fatalf("Data[url] = %v, want %s", result.Data["url"], srv.URL)
	}
}

func TestExtractTitle(t *testing.T) {
	cases := []struct {
		body  string
		title string
	}{
		{`<html><head><title>Hello</title></head></html>`, "Hello"},
		{`<TITLE>Upper Case</TITLE>`, "Upper Case"},
		{`<title>  Spaced  </title>`, "Spaced"},
		{`no title here`, ""},
	}
	for _, c := range cases {
		if got := extractTitle(c.body); got != c.title {
			t.Errorf("extractTitle(%q) = %q, want %q", c.body, got, c.title)
		}
	}
}

func TestCacheKey_deterministicAndDistinct(t *testing.T) {
	k1 := cacheKey("https://example.com/a")
	k2 := cacheKey("https://example.com/a")
	k3 := cacheKey("https://example.com/b")
	if k1 != k2 {
		t.Fatal("same URL produced different cache keys")
	}
	if k1 == k3 {
		t.Fatal("different URLs produced the same cache key")
	}
}

func strContains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

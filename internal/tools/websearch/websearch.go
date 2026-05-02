// Package websearch implements the built-in web search tool (PRD §6.25.7).
//
// Three operating modes:
//   - Cached (default): results are fetched once and stored in
//     ~/.conduit/cache/websearch/ keyed by SHA256 of the URL.
//   - Live: every call fetches fresh content from the network.
//   - Disabled: every call returns an error — useful for offline sessions.
//
// Only GET, HEAD, and OPTIONS HTTP methods are permitted. POST, PUT, and
// DELETE are blocked unconditionally.
package websearch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/tools"
)

// Mode controls whether results come from the local cache, the live network,
// or are disabled entirely.
type Mode string

const (
	ModeCached   Mode = "cached"
	ModeLive     Mode = "live"
	ModeDisabled Mode = "disabled"
)

// Config holds the web search tool's runtime configuration.
type Config struct {
	// Mode is "cached" (default), "live", or "disabled".
	Mode Mode
	// CacheDir overrides the default ~/.conduit/cache/websearch/ location.
	CacheDir string
	// BodyLimit caps the number of bytes kept from the response body.
	// Zero means the package default (8192).
	BodyLimit int
}

// Result is the structured output from one web search call.
type Result struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Body    string `json:"body"`
}

// input is the JSON schema for the tool's call arguments.
type input struct {
	URL    string `json:"url"`
	Method string `json:"method,omitempty"`
}

const (
	defaultBodyLimit  = 8192
	snippetLimit      = 500
	cacheFileExt      = ".json"
	httpClientTimeout = 15 * time.Second
)

// blockedMethods are HTTP methods the tool refuses to issue.
var blockedMethods = map[string]bool{
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodDelete: true,
	http.MethodPatch:  true,
}

// New returns a tools.Tool backed by the web search implementation.
// cfg is applied as-is; call DefaultConfig() to obtain a config with
// sensible defaults already set.
func New(cfg Config) tools.Tool {
	ws := &webSearch{cfg: applyDefaults(cfg)}
	return tools.Tool{
		Name:        "web_search",
		Description: "Fetch content from a URL. Cached mode returns stored results; live always fetches fresh; disabled returns an error. Only GET, HEAD, and OPTIONS are permitted.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch.",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "HTTP method: GET (default), HEAD, or OPTIONS.",
					"enum":        []string{"GET", "HEAD", "OPTIONS"},
				},
			},
			"required": []string{"url"},
		},
		Run: ws.run,
	}
}

// DefaultConfig returns a Config with the package defaults applied.
func DefaultConfig() Config {
	return applyDefaults(Config{})
}

func applyDefaults(cfg Config) Config {
	if cfg.Mode == "" {
		cfg.Mode = ModeCached
	}
	if cfg.BodyLimit == 0 {
		cfg.BodyLimit = defaultBodyLimit
	}
	if cfg.CacheDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.CacheDir = filepath.Join(home, ".conduit", "cache", "websearch")
		}
	}
	return cfg
}

type webSearch struct {
	cfg Config
}

func (ws *webSearch) run(ctx context.Context, raw json.RawMessage) (tools.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return tools.Result{IsError: true, Text: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if in.URL == "" {
		return tools.Result{IsError: true, Text: "url is required"}, nil
	}
	if in.Method == "" {
		in.Method = http.MethodGet
	}
	in.Method = strings.ToUpper(in.Method)

	if blockedMethods[in.Method] {
		return tools.Result{IsError: true, Text: fmt.Sprintf("method %s is not permitted; only GET, HEAD, and OPTIONS are allowed", in.Method)}, nil
	}

	if ws.cfg.Mode == ModeDisabled {
		return tools.Result{IsError: true, Text: "web search is disabled"}, nil
	}

	if ws.cfg.Mode == ModeCached {
		if cached, ok := ws.loadCache(in.URL); ok {
			return resultToToolResult(cached), nil
		}
	}

	fetched, err := ws.fetch(ctx, in.URL, in.Method)
	if err != nil {
		return tools.Result{IsError: true, Text: fmt.Sprintf("fetch error: %v", err)}, nil
	}

	if ws.cfg.Mode == ModeCached {
		_ = ws.saveCache(in.URL, fetched)
	}

	return resultToToolResult(fetched), nil
}

func (ws *webSearch) fetch(ctx context.Context, rawURL, method string) (Result, error) {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return Result{}, fmt.Errorf("invalid URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", "conduit-web-search/1.0")

	client := &http.Client{Timeout: httpClientTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	var bodyBytes []byte
	if method != http.MethodHead && method != http.MethodOptions {
		limited := io.LimitReader(resp.Body, int64(ws.cfg.BodyLimit))
		bodyBytes, err = io.ReadAll(limited)
		if err != nil {
			return Result{}, err
		}
	}

	body := string(bodyBytes)
	title := extractTitle(body)
	snippet := extractSnippet(body)

	return Result{
		URL:     rawURL,
		Title:   title,
		Snippet: snippet,
		Body:    body,
	}, nil
}

// cacheKey returns the SHA256 hex of the URL, used as the cache filename.
func cacheKey(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("%x", sum)
}

func (ws *webSearch) loadCache(rawURL string) (Result, bool) {
	if ws.cfg.CacheDir == "" {
		return Result{}, false
	}
	path := filepath.Join(ws.cfg.CacheDir, cacheKey(rawURL)+cacheFileExt)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Result{}, false
	}
	if err != nil {
		return Result{}, false
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return Result{}, false
	}
	return r, true
}

func (ws *webSearch) saveCache(rawURL string, r Result) error {
	if ws.cfg.CacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(ws.cfg.CacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	path := filepath.Join(ws.cfg.CacheDir, cacheKey(rawURL)+cacheFileExt)
	return os.WriteFile(path, data, 0o644)
}

func resultToToolResult(r Result) tools.Result {
	parts := []string{}
	if r.Title != "" {
		parts = append(parts, "Title: "+r.Title)
	}
	if r.URL != "" {
		parts = append(parts, "URL: "+r.URL)
	}
	if r.Snippet != "" {
		parts = append(parts, "Snippet: "+r.Snippet)
	}
	if r.Body != "" {
		parts = append(parts, "\n"+r.Body)
	}
	return tools.Result{
		Text: strings.Join(parts, "\n"),
		Data: map[string]any{
			"url":     r.URL,
			"title":   r.Title,
			"snippet": r.Snippet,
			"body":    r.Body,
		},
	}
}

var titleRegexp = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)

func extractTitle(body string) string {
	if m := titleRegexp.FindStringSubmatch(body); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

var tagRegexp = regexp.MustCompile(`<[^>]+>`)

func extractSnippet(body string) string {
	text := tagRegexp.ReplaceAllString(body, " ")
	// Collapse runs of whitespace.
	fields := strings.Fields(text)
	joined := strings.Join(fields, " ")
	if len(joined) > snippetLimit {
		return joined[:snippetLimit]
	}
	return joined
}

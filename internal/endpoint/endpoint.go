// Package endpoint provides adapters for external AI provider APIs and the
// connection-validation logic required before an endpoint is saved to config.
package endpoint

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const probeTimeout = 10 * time.Second

// Type identifies the API wire format an external endpoint speaks.
type Type string

const (
	// TypeOpenAICompat covers OpenAI, vLLM, text-generation-inference,
	// OpenRouter, LiteLLM Proxy, and any other server that speaks the
	// OpenAI chat completions format.
	TypeOpenAICompat Type = "openai_compatible"

	// TypeAnthropic covers the Anthropic Messages API.
	TypeAnthropic Type = "anthropic"
)

// Config holds the connection parameters for one external provider.
type Config struct {
	Name    string
	Type    Type
	BaseURL string
	APIKey  string
	Model   string
}

// Validate probes the endpoint with a lightweight request and returns a
// descriptive error if connectivity or authentication fails.
// The connection must succeed within probeTimeout regardless of the parent ctx.
func Validate(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	switch cfg.Type {
	case TypeAnthropic:
		return probeAnthropic(ctx, cfg)
	default:
		return probeOpenAICompat(ctx, cfg)
	}
}

func probeOpenAICompat(ctx context.Context, cfg Config) error {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build probe: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", cfg.BaseURL, err)
	}
	defer resp.Body.Close()
	return interpretStatus(cfg.BaseURL, resp.StatusCode)
}

func probeAnthropic(ctx context.Context, cfg Config) error {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	url := strings.TrimRight(base, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build probe: %w", err)
	}
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", base, err)
	}
	defer resp.Body.Close()
	return interpretStatus(base, resp.StatusCode)
}

func interpretStatus(base string, code int) error {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fmt.Errorf("authentication failed (HTTP %d): check your API key for %s", code, base)
	case code >= 300:
		return fmt.Errorf("%s returned HTTP %d", base, code)
	}
	return nil
}

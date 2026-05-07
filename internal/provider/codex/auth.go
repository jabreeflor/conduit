// Package codex talks to OpenAI's Codex backend (the one behind the official
// codex CLI) using ChatGPT-account credentials.
//
// Auth model: conduit reads the credentials produced by `codex login` from
// ~/.codex/auth.json. Run `codex login` once via the official CLI; conduit
// handles token refresh from there. We deliberately do not reimplement the
// browser/device-code OAuth flow — it's the official CLI's job and would add
// a maintenance liability without user benefit.
package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// clientID is published in the open-source Codex CLI as the public OAuth
	// client identifier — see codex-rs/login/src/auth/manager.rs.
	clientID = "app_EMoamEEZ73f0CkXaXp7hrann"

	// refreshTokenURL is the OAuth token endpoint. Refresh requests are JSON,
	// not form-encoded — see codex-rs/login/src/auth/manager.rs.
	refreshTokenURL = "https://auth.openai.com/oauth/token"

	// refreshInterval mirrors TOKEN_REFRESH_INTERVAL=8 days in the Codex CLI.
	// Tokens are eagerly refreshed once they're older than this, in addition
	// to the JWT-exp check.
	refreshInterval = 8 * 24 * time.Hour
)

// Tokens is the on-disk shape of ~/.codex/auth.json's "tokens" field.
type Tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id,omitempty"`
}

// authFile mirrors ~/.codex/auth.json. Only the fields conduit needs are
// modeled; unknown fields round-trip via rawExtra so writes don't drop them.
type authFile struct {
	AuthMode     string          `json:"auth_mode"`
	OpenAIAPIKey *string         `json:"OPENAI_API_KEY,omitempty"`
	Tokens       *Tokens         `json:"tokens,omitempty"`
	LastRefresh  time.Time       `json:"last_refresh"`
	rawExtra     json.RawMessage // unused fields preserved on round-trip (future-proofing)
}

// Auth is the runtime token store. It loads from disk, refreshes when needed,
// and writes back. Safe for concurrent use.
type Auth struct {
	path string

	mu         sync.Mutex
	file       authFile
	httpClient *http.Client
	now        func() time.Time // injectable for tests
}

// LoadAuth reads ~/.codex/auth.json (or $CODEX_HOME/auth.json) and returns an
// Auth that can be used to obtain access tokens.
func LoadAuth() (*Auth, error) {
	path, err := defaultAuthPath()
	if err != nil {
		return nil, err
	}
	return LoadAuthFromPath(path)
}

// LoadAuthFromPath reads a specific auth.json. Useful for tests and for
// callers that override the location.
func LoadAuthFromPath(path string) (*Auth, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("codex: %s not found — run `codex login` first", path)
		}
		return nil, fmt.Errorf("codex: read %s: %w", path, err)
	}
	var f authFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("codex: parse %s: %w", path, err)
	}
	if f.Tokens == nil || f.Tokens.AccessToken == "" {
		return nil, fmt.Errorf("codex: %s has no access token — run `codex login` first", path)
	}
	return &Auth{
		path:       path,
		file:       f,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}, nil
}

// AccessToken returns a valid access token, refreshing first if needed.
func (a *Auth) AccessToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.maybeRefreshLocked(ctx); err != nil {
		return "", err
	}
	return a.file.Tokens.AccessToken, nil
}

// AccountID returns the ChatGPT-Account-ID header value, or "" if not set.
func (a *Auth) AccountID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file.Tokens == nil {
		return ""
	}
	return a.file.Tokens.AccountID
}

// AuthMode reports the current mode — "Chatgpt", "ApiKey", etc.
func (a *Auth) AuthMode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.file.AuthMode
}

// maybeRefreshLocked refreshes the access token if (a) the JWT exp claim is
// in the past, or (b) last_refresh is older than refreshInterval. The mutex
// must already be held.
func (a *Auth) maybeRefreshLocked(ctx context.Context) error {
	if a.file.Tokens == nil {
		return fmt.Errorf("codex: no tokens loaded")
	}
	if a.file.AuthMode == "ApiKey" {
		// API-key mode doesn't use refresh.
		return nil
	}

	now := a.now()
	exp, err := jwtExp(a.file.Tokens.AccessToken)
	expired := err == nil && now.After(exp)
	stale := !a.file.LastRefresh.IsZero() && now.Sub(a.file.LastRefresh) > refreshInterval
	if !expired && !stale {
		return nil
	}

	return a.refreshLocked(ctx)
}

// refreshLocked POSTs the refresh request and persists the new tokens.
func (a *Auth) refreshLocked(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"refresh_token": a.file.Tokens.RefreshToken,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshTokenURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("codex: refresh request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("codex: refresh HTTP %d", resp.StatusCode)
	}
	var out struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("codex: decode refresh response: %w", err)
	}
	if out.AccessToken != "" {
		a.file.Tokens.AccessToken = out.AccessToken
	}
	if out.IDToken != "" {
		a.file.Tokens.IDToken = out.IDToken
	}
	if out.RefreshToken != "" {
		a.file.Tokens.RefreshToken = out.RefreshToken
	}
	a.file.LastRefresh = a.now().UTC()
	return a.writeLocked()
}

// writeLocked persists auth.json. Ownership/permission semantics match what
// `codex login` writes: 0600 on the file.
func (a *Auth) writeLocked() error {
	out, err := json.MarshalIndent(a.file, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("codex: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, a.path); err != nil {
		return fmt.Errorf("codex: rename %s: %w", a.path, err)
	}
	return nil
}

// jwtExp decodes the "exp" claim from a JWT without verifying the signature.
// We only use it to decide when to proactively refresh; the server is the
// final authority on validity.
func jwtExp(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some JWT serializers include padding; tolerate that.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("decode payload: %w", err)
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, err
	}
	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("no exp claim")
	}
	return time.Unix(claims.Exp, 0), nil
}

// defaultAuthPath returns $CODEX_HOME/auth.json or ~/.codex/auth.json.
func defaultAuthPath() (string, error) {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return filepath.Join(h, "auth.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

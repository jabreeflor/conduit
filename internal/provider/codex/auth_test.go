package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeJWT builds a JWT with the given exp claim. Signature is unverified by
// our parser, so the third segment can be anything.
func makeJWT(exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]any{"exp": exp.Unix()})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func writeAuthFile(t *testing.T, dir string, f authFile) string {
	t.Helper()
	path := filepath.Join(dir, "auth.json")
	out, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAuthFromPath_MissingFile(t *testing.T) {
	_, err := LoadAuthFromPath(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil || !strings.Contains(err.Error(), "codex login") {
		t.Fatalf("want hint about codex login, got %v", err)
	}
}

func TestLoadAuthFromPath_NoTokens(t *testing.T) {
	dir := t.TempDir()
	path := writeAuthFile(t, dir, authFile{AuthMode: "Chatgpt"})
	_, err := LoadAuthFromPath(path)
	if err == nil || !strings.Contains(err.Error(), "no access token") {
		t.Fatalf("want no-token error, got %v", err)
	}
}

func TestAccessToken_NoRefreshWhenFresh(t *testing.T) {
	dir := t.TempDir()
	tok := makeJWT(time.Now().Add(time.Hour))
	path := writeAuthFile(t, dir, authFile{
		AuthMode:    "Chatgpt",
		Tokens:      &Tokens{AccessToken: tok, RefreshToken: "r1", AccountID: "acc"},
		LastRefresh: time.Now().Add(-time.Hour),
	})
	a, err := LoadAuthFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != tok {
		t.Errorf("access token: got %q want %q", got, tok)
	}
	if a.AccountID() != "acc" {
		t.Errorf("account id: got %q", a.AccountID())
	}
}

func TestAccessToken_RefreshesWhenExpired(t *testing.T) {
	expired := makeJWT(time.Now().Add(-time.Hour))
	fresh := makeJWT(time.Now().Add(time.Hour))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" {
			t.Errorf("grant_type: got %q want refresh_token", body["grant_type"])
		}
		if body["client_id"] != clientID {
			t.Errorf("client_id: got %q want %q", body["client_id"], clientID)
		}
		if body["refresh_token"] != "r1" {
			t.Errorf("refresh_token: got %q", body["refresh_token"])
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type: got %q", r.Header.Get("Content-Type"))
		}
		fmt.Fprintf(w, `{"id_token":"new_id","access_token":%q,"refresh_token":"r2"}`, fresh)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := writeAuthFile(t, dir, authFile{
		AuthMode: "Chatgpt",
		Tokens:   &Tokens{AccessToken: expired, RefreshToken: "r1", AccountID: "acc", IDToken: "old"},
	})
	a, err := LoadAuthFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	// Redirect refresh to the test server. Production code uses the const URL;
	// for the test we monkey-patch via a bound httpClient that targets srv.
	a.httpClient = srv.Client()
	a.httpClient.Transport = &rewriteToServer{base: srv.URL}

	got, err := a.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != fresh {
		t.Errorf("access token: got %q want %q", got, fresh)
	}

	// Round-trip: file on disk should now hold the new tokens.
	raw, _ := os.ReadFile(path)
	var f authFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Tokens.AccessToken != fresh {
		t.Errorf("persisted access token: got %q want %q", f.Tokens.AccessToken, fresh)
	}
	if f.Tokens.RefreshToken != "r2" {
		t.Errorf("persisted refresh token: got %q want r2", f.Tokens.RefreshToken)
	}
	if f.Tokens.IDToken != "new_id" {
		t.Errorf("persisted id token: got %q want new_id", f.Tokens.IDToken)
	}
	if f.LastRefresh.IsZero() {
		t.Error("last_refresh not stamped")
	}
}

func TestAccessToken_ApiKeyModeSkipsRefresh(t *testing.T) {
	dir := t.TempDir()
	expired := makeJWT(time.Now().Add(-time.Hour))
	path := writeAuthFile(t, dir, authFile{
		AuthMode: "ApiKey",
		Tokens:   &Tokens{AccessToken: expired, RefreshToken: "r1"},
	})
	a, err := LoadAuthFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != expired {
		t.Errorf("ApiKey mode should not refresh; got %q want %q", got, expired)
	}
}

func TestJwtExp(t *testing.T) {
	want := time.Unix(1234567890, 0)
	exp, err := jwtExp(makeJWT(want))
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(want) {
		t.Errorf("exp: got %v want %v", exp, want)
	}
	if _, err := jwtExp("not.a.jwt"); err == nil {
		// "not.a.jwt" has 3 segments; the middle isn't valid base64. Should error.
		t.Error("expected error on bad payload")
	}
}

// rewriteToServer rewrites every outbound request to the test server's host.
type rewriteToServer struct{ base string }

func (r *rewriteToServer) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace scheme + host with the test server's. Path is preserved.
	target := r.base + req.URL.Path
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, target, req.Body)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Header {
		newReq.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(newReq)
}

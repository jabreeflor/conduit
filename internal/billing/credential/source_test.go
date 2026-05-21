package credential_test

import (
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/billing/credential"
)

func TestLoadFromEnv_MultiKey(t *testing.T) {
	t.Setenv("CONDUIT_TESTPROVIDER_API_KEY_1", "k1")
	t.Setenv("CONDUIT_TESTPROVIDER_API_KEY_2", "k2")

	p := credential.LoadFromEnv("testprovider", 0)
	if got := p.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
}

func TestLoadFromEnv_SingleKeyFallback(t *testing.T) {
	t.Setenv("CONDUIT_TESTPROVIDER2_API_KEY", "solo")

	p := credential.LoadFromEnv("testprovider2", 0)
	if got := p.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	k, ok := p.Next()
	if !ok || k != "solo" {
		t.Errorf("Next() = (%q, %v), want (\"solo\", true)", k, ok)
	}
}

func TestLoadFromEnv_NoKeys(t *testing.T) {
	p := credential.LoadFromEnv("nonexistent_xyz_provider", time.Minute)
	if got := p.Len(); got != 0 {
		t.Fatalf("Len() = %d, want 0", got)
	}
}

func TestLoadFromEnv_AnthropicAcceptsClaudeCodeToken(t *testing.T) {
	// Unset ANTHROPIC_API_KEY so the Claude Code alias is the only credential.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "subscription-token")

	p := credential.LoadFromEnv("anthropic", 0)
	if p.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 from CLAUDE_CODE_OAUTH_TOKEN", p.Len())
	}
	k, ok := p.Next()
	if !ok || k != "subscription-token" {
		t.Errorf("Next() = (%q, %v), want (\"subscription-token\", true)", k, ok)
	}
}

func TestLoadFromEnv_AnthropicPrefersExplicitConduitKey(t *testing.T) {
	t.Setenv("CONDUIT_ANTHROPIC_API_KEY", "explicit")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "fallback")

	p := credential.LoadFromEnv("anthropic", 0)
	if p.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", p.Len())
	}
	k, _ := p.Next()
	if k != "explicit" {
		t.Errorf("Next() = %q, want explicit CONDUIT_ key to win over Claude Code fallback", k)
	}
}

func TestLoadFromEnv_CaseInsensitive(t *testing.T) {
	t.Setenv("CONDUIT_MYPROVIDER_API_KEY", "lower")

	lower := credential.LoadFromEnv("myprovider", 0)
	upper := credential.LoadFromEnv("MYPROVIDER", 0)

	if lower.Len() != 1 || upper.Len() != 1 {
		t.Errorf("both casings should resolve to 1 key; lower=%d upper=%d", lower.Len(), upper.Len())
	}
}

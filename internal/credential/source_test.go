package credential_test

import (
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/credential"
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

func TestLoadFromEnv_CaseInsensitive(t *testing.T) {
	t.Setenv("CONDUIT_MYPROVIDER_API_KEY", "lower")

	lower := credential.LoadFromEnv("myprovider", 0)
	upper := credential.LoadFromEnv("MYPROVIDER", 0)

	if lower.Len() != 1 || upper.Len() != 1 {
		t.Errorf("both casings should resolve to 1 key; lower=%d upper=%d", lower.Len(), upper.Len())
	}
}

package credential_test

import (
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/credential"
)

func TestPool_RoundRobin(t *testing.T) {
	p := credential.New([]string{"a", "b", "c"}, 0)
	seen := make(map[string]int)
	for range 6 {
		k, ok := p.Next()
		if !ok {
			t.Fatal("expected a healthy key")
		}
		seen[k]++
	}
	for _, key := range []string{"a", "b", "c"} {
		if seen[key] != 2 {
			t.Errorf("key %q: got %d calls, want 2", key, seen[key])
		}
	}
}

func TestPool_SkipsUnhealthyKey(t *testing.T) {
	p := credential.New([]string{"a", "b"}, 10*time.Minute)
	p.MarkUnhealthy("a")

	k, ok := p.Next()
	if !ok {
		t.Fatal("expected a healthy key")
	}
	if k != "b" {
		t.Errorf("got %q, want \"b\"", k)
	}
}

func TestPool_AllUnhealthy(t *testing.T) {
	p := credential.New([]string{"a", "b"}, 10*time.Minute)
	p.MarkUnhealthy("a")
	p.MarkUnhealthy("b")

	_, ok := p.Next()
	if ok {
		t.Fatal("expected no healthy keys when all are in backoff")
	}
}

func TestPool_BackoffExpiry(t *testing.T) {
	p := credential.New([]string{"a"}, 1*time.Millisecond)
	p.MarkUnhealthy("a")

	if _, ok := p.Next(); ok {
		t.Fatal("key should be within backoff window immediately after MarkUnhealthy")
	}

	time.Sleep(5 * time.Millisecond)

	k, ok := p.Next()
	if !ok {
		t.Fatal("key should have recovered after backoff window expired")
	}
	if k != "a" {
		t.Errorf("got %q, want \"a\"", k)
	}
}

func TestPool_Empty(t *testing.T) {
	p := credential.New(nil, 0)

	if _, ok := p.Next(); ok {
		t.Fatal("empty pool should never return a key")
	}
	if n := p.Len(); n != 0 {
		t.Errorf("Len() = %d, want 0", n)
	}
}

func TestPool_HealthyCount(t *testing.T) {
	p := credential.New([]string{"a", "b", "c"}, 10*time.Minute)

	if n := p.HealthyCount(); n != 3 {
		t.Errorf("HealthyCount() = %d, want 3", n)
	}

	p.MarkUnhealthy("b")

	if n := p.HealthyCount(); n != 2 {
		t.Errorf("HealthyCount() after one unhealthy = %d, want 2", n)
	}
}

func TestPool_MarkUnknownKeyIgnored(t *testing.T) {
	p := credential.New([]string{"a"}, 0)
	p.MarkUnhealthy("nonexistent") // must not panic

	if n := p.HealthyCount(); n != 1 {
		t.Errorf("HealthyCount() = %d, want 1 after marking unknown key", n)
	}
}

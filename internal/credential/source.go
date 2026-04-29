package credential

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LoadFromEnv loads API keys for provider from environment variables.
//
// It first scans CONDUIT_{PROVIDER}_API_KEY_1, _2, … until one is absent,
// then falls back to the bare CONDUIT_{PROVIDER}_API_KEY for a single key.
// provider is case-insensitive ("anthropic", "ANTHROPIC", etc.).
func LoadFromEnv(provider string, backoff time.Duration) *Pool {
	upper := strings.ToUpper(provider)

	var keys []string
	for i := 1; ; i++ {
		k := os.Getenv(fmt.Sprintf("CONDUIT_%s_API_KEY_%d", upper, i))
		if k == "" {
			break
		}
		keys = append(keys, k)
	}

	if len(keys) == 0 {
		if k := os.Getenv(fmt.Sprintf("CONDUIT_%s_API_KEY", upper)); k != "" {
			keys = append(keys, k)
		}
	}

	return New(keys, backoff)
}

// LoadFromKeychain loads API keys for provider from the macOS Keychain.
//
// Keys are stored as generic passwords with service "conduit.{provider}"
// and accounts "key.1", "key.2", … Scanning stops at the first missing
// account. Returns an empty pool on non-macOS platforms or when no entries
// are found.
func LoadFromKeychain(provider string, backoff time.Duration) *Pool {
	service := fmt.Sprintf("conduit.%s", strings.ToLower(provider))

	var keys []string
	for i := 1; ; i++ {
		out, err := exec.Command( //nolint:gosec
			"security", "find-generic-password",
			"-s", service,
			"-a", fmt.Sprintf("key.%d", i),
			"-w",
		).Output()
		if err != nil {
			break
		}
		if k := strings.TrimSpace(string(out)); k != "" {
			keys = append(keys, k)
		}
	}

	return New(keys, backoff)
}

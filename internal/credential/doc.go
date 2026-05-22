// Package credential manages round-robin API-key pools with automatic
// health-check rotation. A key is skipped once marked unhealthy and
// re-enters the rotation when its backoff window expires.
//
// Keys are never stored in plaintext; load them from environment variables
// (LoadFromEnv) or the macOS Keychain (LoadFromKeychain).
package credential

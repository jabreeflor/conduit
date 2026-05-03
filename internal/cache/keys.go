package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Key returns a stable SHA256 cache key for a namespace plus JSON payload.
func Key(namespace string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(namespace+"\x00"), data...))
	return hex.EncodeToString(sum[:]), nil
}

func MustKey(namespace string, payload any) string {
	key, err := Key(namespace, payload)
	if err != nil {
		panic(fmt.Sprintf("cache key: %v", err))
	}
	return key
}

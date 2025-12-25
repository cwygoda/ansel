package nanoid

import (
	"crypto/rand"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// Generate creates a random 8-character ID using lowercase alphanumeric characters.
// The ID has approximately 41 bits of entropy (log2(36^8) ≈ 41.4).
func Generate() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}

	return string(b), nil
}

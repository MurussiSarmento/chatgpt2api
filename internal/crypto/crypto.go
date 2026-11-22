// Package crypto provides authenticated encryption (AES-256-GCM) for sensitive
// data at rest and a deterministic HMAC for indexed lookups. The key is derived
// from the ENCRYPTION_KEY environment value via Init.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
)

// marker prefixes every ciphertext so Decrypt can tell encrypted values from
// legacy plaintext and skip already-encrypted rows during backfill.
const marker = "enc:v1:"

var (
	mu      sync.RWMutex
	gcm     cipher.AEAD
	hmacKey []byte
	ready   bool
)

// Init derives the AES-GCM and HMAC keys from secret. Any-length input is
// accepted; it is hashed to a 32-byte key. Must be called once at startup.
func Init(secret string) error {
	if secret == "" {
		return errors.New("encryption key is empty")
	}
	sum := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	hsum := sha256.Sum256(append([]byte("chatgpt2api-hmac:"), sum[:]...))
	mu.Lock()
	gcm, hmacKey, ready = aead, hsum[:], true
	mu.Unlock()
	return nil
}

// Ready reports whether Init has run.
func Ready() bool {
	mu.RLock()
	defer mu.RUnlock()
	return ready
}

// Encrypt returns a marked, base64-encoded ciphertext. Empty input stays empty
// so callers that test for "" (e.g. WHERE token <> '') keep working.
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	mu.RLock()
	aead, ok := gcm, ready
	mu.RUnlock()
	if !ok {
		return "", errors.New("crypto not initialized")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return marker + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt. Values without the marker are returned unchanged
// (legacy plaintext passthrough during rollout).
func Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, marker) {
		return value, nil
	}
	mu.RLock()
	aead, ok := gcm, ready
	mu.RUnlock()
	if !ok {
		return "", errors.New("crypto not initialized")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, marker))
	if err != nil {
		return "", err
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// IsEncrypted reports whether value is already an Encrypt output.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, marker)
}

// Hash returns a deterministic HMAC-SHA256 hex digest, used as an indexed
// lookup key for values whose plaintext must not be stored in the index.
func Hash(value string) string {
	mu.RLock()
	key := hmacKey
	mu.RUnlock()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

package node

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

const tokenPrefix = "lgn_"

// MintToken returns a new token and the hash node.json stores for it.
func MintToken() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = tokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken is the hex SHA-256 of the whole token, prefix included.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Authenticate returns the live grant a token belongs to, failing alike for unknown, expired and revoked tokens.
func Authenticate(cfg Config, token string, now time.Time) (Grant, bool) {
	if !strings.HasPrefix(token, tokenPrefix) {
		return Grant{}, false
	}
	hash := []byte(HashToken(token))
	var found Grant
	ok := false
	for _, g := range cfg.Grants {
		if subtle.ConstantTimeCompare(hash, []byte(g.TokenHash)) == 1 {
			found, ok = g, true
		}
	}
	if !ok || found.Revoked || !now.Before(found.ExpiresAt) {
		return Grant{}, false
	}
	return found, true
}

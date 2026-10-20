// Package vault keeps credentials safe: the encrypted store, key derivation, stored connections and resters, and channel metadata and keys. It executes nothing on a caller's behalf.
package vault

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
)

// ChannelID is the vault's own high-entropy reference for a channel.
type ChannelID string

type Scope struct {
	Tables     []string
	Operations []string
}

type Channel struct {
	ID        ChannelID
	Name      string
	DBName    string
	Scope     Scope
	Salt      []byte
	Port      int
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (c Channel) Expired(now time.Time) bool {
	return !now.Before(c.ExpiresAt)
}

func (c Channel) Kind() string {
	return "db"
}

func NewChannelID() (ChannelID, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("vault: generating channel id: %w", err)
	}
	return ChannelID(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

func NewSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("vault: generating salt: %w", err)
	}
	return salt, nil
}

var (
	ErrEmptyPassphrase  = errors.New("vault: master passphrase is empty")
	ErrEmptySalt        = errors.New("vault: salt is empty")
	ErrEmptyChannelName = errors.New("vault: channel name is empty")
)

func DeriveKey(masterPassphrase string, salt []byte) ([]byte, error) {
	if masterPassphrase == "" {
		return nil, ErrEmptyPassphrase
	}
	if len(salt) == 0 {
		return nil, ErrEmptySalt
	}
	return argon2.IDKey([]byte(masterPassphrase), salt, argonTime, argonMemory, argonThreads, argonKeyLen), nil
}

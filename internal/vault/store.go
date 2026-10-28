// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Store persists arbitrary values at rest, one AES-256-GCM-encrypted file per name, under a directory the caller owns.
type Store struct {
	dir string
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("vault: creating %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

var ErrNotFound = errors.New("vault: not found")

// ErrWrongKey means a stored entry failed to decrypt under the supplied key.
var ErrWrongKey = errors.New("vault: wrong key or corrupt data")

func (s *Store) Put(name string, key, value []byte) error {
	gcm, err := newGCM(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("vault: generating nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, value, nil)
	return os.WriteFile(s.path(name), sealed, 0o600)
}

// Get relies on GCM authentication failing for a wrong key rather than a separate stored verifier.
func (s *Store) Get(name string, key []byte) ([]byte, error) {
	sealed, err := os.ReadFile(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("vault: reading %s: %w", name, err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, fmt.Errorf("vault: %s is corrupt", name)
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w for %s: %w", ErrWrongKey, name, err)
	}
	return plain, nil
}

func (s *Store) Delete(name string) error {
	err := os.Remove(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// List returns every stored name, undecrypted -- the filename itself carries no key material.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("vault: reading %s: %w", s.dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".enc" {
			continue
		}
		names = append(names, e.Name()[:len(e.Name())-len(".enc")])
	}
	return names, nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name+".enc")
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("vault: building cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault: building AEAD: %w", err)
	}
	return gcm, nil
}

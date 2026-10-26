// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Service never holds a root secret as a field, only a channel's own derived key cached in memory while that channel is active.
type Service struct {
	creds       *Store
	channels    *Store
	canary      *Store
	policies    *Store
	metaDir     string
	httpMetaDir string
	rootSalt    []byte

	// resterMu makes PutRester's name check and write one step.
	resterMu sync.Mutex

	mu         sync.Mutex
	activeKeys map[ChannelID][]byte
	onInvalid  []func(ChannelID)

	policyPath string
	policy     policyFile
}

func NewService(dir string) (*Service, error) {
	creds, err := OpenStore(filepath.Join(dir, "creds"))
	if err != nil {
		return nil, err
	}
	channels, err := OpenStore(filepath.Join(dir, "channels"))
	if err != nil {
		return nil, err
	}
	canary, err := OpenStore(filepath.Join(dir, "canary"))
	if err != nil {
		return nil, err
	}
	policies, err := OpenStore(filepath.Join(dir, "policy"))
	if err != nil {
		return nil, err
	}
	metaDir := filepath.Join(dir, "meta")
	if err := os.MkdirAll(metaDir, 0o700); err != nil {
		return nil, fmt.Errorf("vault: creating %s: %w", metaDir, err)
	}
	httpMetaDir := filepath.Join(dir, "meta-http")
	if err := os.MkdirAll(httpMetaDir, 0o700); err != nil {
		return nil, fmt.Errorf("vault: creating %s: %w", httpMetaDir, err)
	}
	rootSalt, err := loadOrCreateRootSalt(filepath.Join(dir, "root.salt"))
	if err != nil {
		return nil, err
	}
	policyPath := filepath.Join(dir, policyFileName)
	return &Service{
		creds:       creds,
		channels:    channels,
		canary:      canary,
		policies:    policies,
		metaDir:     metaDir,
		httpMetaDir: httpMetaDir,
		rootSalt:    rootSalt,
		activeKeys:  make(map[ChannelID][]byte),
		policyPath:  policyPath,
		policy:      loadPolicies(policyPath),
	}, nil
}

func loadOrCreateRootSalt(path string) ([]byte, error) {
	salt, err := os.ReadFile(path)
	if err == nil {
		return salt, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("vault: reading %s: %w", path, err)
	}
	salt, err = NewSalt()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, salt, 0o600); err != nil {
		return nil, fmt.Errorf("vault: writing %s: %w", path, err)
	}
	return salt, nil
}

// PutCredential encrypts value under the root key and stores it as dbName.
func (s *Service) PutCredential(rootSecret, dbName string, value []byte) error {
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)
	return s.creds.Put(dbName, rootKey, value)
}

// ListConnections lists stored credential names only, never a decrypted value, without needing
// a root secret. Excludes rester entries, which share this same Store under their own key prefix.
func (s *Service) ListConnections() ([]string, error) {
	names, err := s.creds.List()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if strings.HasPrefix(n, resterKeyPrefix) {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

const (
	canaryEntryName = "verify"
	canaryPlaintext = "lemongrass"
)

var ErrWrongPassphrase = errors.New("vault: wrong passphrase")

// HasPassphrase reports whether a vault passphrase has ever been set, independent of whether any connection currently exists.
func (s *Service) HasPassphrase() (bool, error) {
	names, err := s.canary.List()
	if err != nil {
		return false, err
	}
	return len(names) > 0, nil
}

// SetPassphrase records rootSecret as the vault's passphrase, meaningful only the first time, so callers must check HasPassphrase first.
func (s *Service) SetPassphrase(rootSecret string) error {
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)
	return s.canary.Put(canaryEntryName, rootKey, []byte(canaryPlaintext))
}

// VerifyPassphrase checks rootSecret against the recorded canary, never touching a real connection, so a failure here always means the passphrase is wrong.
func (s *Service) VerifyPassphrase(rootSecret string) error {
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)
	plain, err := s.canary.Get(canaryEntryName, rootKey)
	if err != nil {
		return ErrWrongPassphrase
	}
	defer Zero(plain)
	if string(plain) != canaryPlaintext {
		return ErrWrongPassphrase
	}
	return nil
}

// ResetVault permanently discards the canary and every stored credential, rester, and channel
// of both kinds. The only way back from a forgotten passphrase, since nothing encrypted under
// it is recoverable without it. The policy file is kept: it is not sealed under the passphrase,
// and a reset, which needs no passphrase, must not drop the user's own denials.
func (s *Service) ResetVault() error {
	channels, err := s.ListChannels()
	if err != nil {
		return err
	}
	for _, c := range channels {
		if err := s.Revoke(c.ID); err != nil {
			return err
		}
	}
	httpChannels, err := s.ListHTTPChannels()
	if err != nil {
		return err
	}
	for _, c := range httpChannels {
		if err := s.RevokeHTTP(c.ID); err != nil {
			return err
		}
	}
	names, err := s.creds.List()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := s.creds.Delete(name); err != nil {
			return err
		}
	}
	if err := s.policies.Delete(sealedPolicyEntryName); err != nil {
		return err
	}
	return s.canary.Delete(canaryEntryName)
}

// DeleteConnection removes a stored credential. It succeeds even if dbName was never stored.
func (s *Service) DeleteConnection(dbName string) error {
	return s.creds.Delete(dbName)
}

// CreateChannel decrypts dbName's root-encrypted credential once and stores a separately-encrypted copy under a new channel id, wrapped with that channel's own derived key.
func (s *Service) CreateChannel(rootSecret, name, dbName string, scope Scope, ttl time.Duration) (Channel, error) {
	if name == "" {
		return Channel{}, ErrEmptyChannelName
	}

	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return Channel{}, err
	}
	defer Zero(rootKey)

	plain, err := s.creds.Get(dbName, rootKey)
	if err != nil {
		return Channel{}, fmt.Errorf("vault: reading credential for %s: %w", dbName, err)
	}
	defer Zero(plain)

	id, err := NewChannelID()
	if err != nil {
		return Channel{}, err
	}
	salt, err := NewSalt()
	if err != nil {
		return Channel{}, err
	}
	channelKey, err := DeriveKey(rootSecret, salt)
	if err != nil {
		return Channel{}, err
	}
	locked, err := LockKey(channelKey)
	if err != nil {
		Zero(channelKey)
		return Channel{}, err
	}

	port, err := s.allocatePort()
	if err != nil {
		LockedFree(locked)
		return Channel{}, err
	}

	if err := s.channels.Put(string(id), locked, plain); err != nil {
		LockedFree(locked)
		return Channel{}, fmt.Errorf("vault: wrapping credential for channel %s: %w", id, err)
	}

	now := time.Now()
	c := Channel{
		ID:        id,
		Name:      name,
		DBName:    dbName,
		Scope:     scope,
		Salt:      salt,
		Port:      port,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.saveMeta(c); err != nil {
		LockedFree(locked)
		s.channels.Delete(string(id))
		return Channel{}, err
	}

	s.mu.Lock()
	s.activeKeys[id] = locked
	s.mu.Unlock()
	return c, nil
}

// Activate re-derives an existing channel's key and refreshes its TTL, without decrypting the root-encrypted base credential again.
func (s *Service) Activate(rootSecret string, id ChannelID, ttl time.Duration) (Channel, error) {
	c, err := s.loadMeta(id)
	if err != nil {
		return Channel{}, err
	}
	channelKey, err := DeriveKey(rootSecret, c.Salt)
	if err != nil {
		return Channel{}, err
	}
	locked, err := LockKey(channelKey)
	if err != nil {
		Zero(channelKey)
		return Channel{}, err
	}

	c.ExpiresAt = time.Now().Add(ttl)
	if err := s.saveMeta(c); err != nil {
		LockedFree(locked)
		return Channel{}, err
	}

	s.mu.Lock()
	if old, ok := s.activeKeys[id]; ok {
		LockedFree(old)
	}
	s.activeKeys[id] = locked
	s.mu.Unlock()
	return c, nil
}

// Revoke removes a channel's metadata, wrapped credential, cached key, succeeding even if the channel was never active.
func (s *Service) Revoke(id ChannelID) error {
	s.forgetKey(id)

	if err := s.channels.Delete(string(id)); err != nil {
		return err
	}
	return s.deleteMeta(id)
}

// forgetKey releases id's cached channel key on revocation or TTL expiry, so a locked key's memory doesn't stay resident past the point it's usable.
func (s *Service) forgetKey(id ChannelID) {
	s.mu.Lock()
	if key, ok := s.activeKeys[id]; ok {
		LockedFree(key)
		delete(s.activeKeys, id)
	}
	s.mu.Unlock()
	s.invalidate(id)
}

// ChannelScope returns a channel's metadata without needing the channel to be active.
func (s *Service) ChannelScope(id ChannelID) (Channel, error) {
	return s.loadMeta(id)
}

// ListChannels reads every channel's metadata off disk, since metaDir is the source of truth rather than an in-memory index, newest first.
func (s *Service) ListChannels() ([]Channel, error) {
	entries, err := os.ReadDir(s.metaDir)
	if err != nil {
		return nil, fmt.Errorf("vault: reading %s: %w", s.metaDir, err)
	}
	channels := make([]Channel, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := ChannelID(e.Name()[:len(e.Name())-len(".json")])
		c, err := s.loadMeta(id)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	slices.SortFunc(channels, func(a, b Channel) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return channels, nil
}

func (s *Service) saveMeta(c Channel) error {
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("vault: encoding metadata for channel %s: %w", c.ID, err)
	}
	return os.WriteFile(s.metaPath(c.ID), b, 0o600)
}

func (s *Service) loadMeta(id ChannelID) (Channel, error) {
	b, err := os.ReadFile(s.metaPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return Channel{}, ErrNotFound
	}
	if err != nil {
		return Channel{}, fmt.Errorf("vault: reading metadata for channel %s: %w", id, err)
	}
	var c Channel
	if err := json.Unmarshal(b, &c); err != nil {
		return Channel{}, fmt.Errorf("vault: decoding metadata for channel %s: %w", id, err)
	}
	return c, nil
}

func (s *Service) deleteMeta(id ChannelID) error {
	err := os.Remove(s.metaPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Service) metaPath(id ChannelID) string {
	return filepath.Join(s.metaDir, string(id)+".json")
}

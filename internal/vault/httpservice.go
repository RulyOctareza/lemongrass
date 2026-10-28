// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// CreateHTTPChannel decrypts resterName's root-encrypted config once, resolves it for one
// environment and type, and stores that flat copy under a new channel id, wrapped with that
// channel's own derived key -- the same pattern CreateChannel uses for a db credential. The scope
// that does not match typ is dropped. It refuses an environment or side that does not exist, an
// environment with no address for typ, and a channel no user could use.
func (s *Service) CreateHTTPChannel(rootSecret, name, resterName, environment, typ string, scope HTTPScope, webScope *WebScope, ttl time.Duration) (HTTPChannel, error) {
	if name == "" {
		return HTTPChannel{}, ErrEmptyChannelName
	}
	if environment == "" {
		return HTTPChannel{}, errors.New("vault: a channel needs an environment")
	}
	switch typ {
	case TypeAPI:
		webScope = nil
	case TypeApp:
		scope, webScope = HTTPScope{}, nil
	case TypeWeb:
		scope = HTTPScope{}
		if webScope == nil {
			webScope = &WebScope{}
		}
		if err := webScope.Validate(); err != nil {
			return HTTPChannel{}, err
		}
	default:
		return HTTPChannel{}, fmt.Errorf("vault: unknown channel type %q, want %q, %q or %q", typ, TypeAPI, TypeWeb, TypeApp)
	}

	// Held until the metadata is saved so UpdateRester, which checks a rester's channels under the same lock, sees this one.
	s.resterMu.Lock()
	defer s.resterMu.Unlock()
	rester, err := s.GetRester(rootSecret, resterName)
	if err != nil {
		return HTTPChannel{}, err
	}
	resolved, err := rester.resolve(environment, typ)
	if err != nil {
		return HTTPChannel{}, err
	}
	if len(resolved.Users) == 0 {
		return HTTPChannel{}, fmt.Errorf("vault: no user of rester %s can use a %s channel in environment %q", resterName, typ, environment)
	}
	plain, err := json.Marshal(resolved)
	if err != nil {
		return HTTPChannel{}, fmt.Errorf("vault: encoding rester %s: %w", resterName, err)
	}
	defer Zero(plain)

	id, err := NewChannelID()
	if err != nil {
		return HTTPChannel{}, err
	}
	salt, err := NewSalt()
	if err != nil {
		return HTTPChannel{}, err
	}
	channelKey, err := DeriveKey(rootSecret, salt)
	if err != nil {
		return HTTPChannel{}, err
	}
	locked, err := LockKey(channelKey)
	if err != nil {
		Zero(channelKey)
		return HTTPChannel{}, err
	}

	if err := s.channels.Put(string(id), locked, plain); err != nil {
		LockedFree(locked)
		return HTTPChannel{}, fmt.Errorf("vault: wrapping rester for channel %s: %w", id, err)
	}

	now := time.Now()
	c := HTTPChannel{
		ID:          id,
		Name:        name,
		Rester:      resterName,
		Environment: environment,
		Type:        typ,
		Scope:       scope,
		WebScope:    webScope,
		Salt:        salt,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
	}
	if err := s.saveHTTPMeta(c); err != nil {
		LockedFree(locked)
		s.channels.Delete(string(id))
		return HTTPChannel{}, err
	}

	s.mu.Lock()
	s.activeKeys[id] = locked
	s.mu.Unlock()
	return c, nil
}

// ActivateHTTP re-derives an existing HTTP channel's key and refreshes its TTL, without
// decrypting the root-encrypted rester again.
func (s *Service) ActivateHTTP(rootSecret string, id ChannelID, ttl time.Duration) (HTTPChannel, error) {
	c, err := s.loadHTTPMeta(id)
	if err != nil {
		return HTTPChannel{}, err
	}
	channelKey, err := DeriveKey(rootSecret, c.Salt)
	if err != nil {
		return HTTPChannel{}, err
	}
	locked, err := LockKey(channelKey)
	if err != nil {
		Zero(channelKey)
		return HTTPChannel{}, err
	}

	c.ExpiresAt = time.Now().Add(ttl)
	if err := s.saveHTTPMeta(c); err != nil {
		LockedFree(locked)
		return HTTPChannel{}, err
	}

	s.mu.Lock()
	if old, ok := s.activeKeys[id]; ok {
		LockedFree(old)
	}
	s.activeKeys[id] = locked
	s.mu.Unlock()
	return c, nil
}

// RevokeHTTP removes an HTTP channel's metadata, wrapped rester copy, and cached key,
// succeeding even if the channel was never active.
func (s *Service) RevokeHTTP(id ChannelID) error {
	s.forgetKey(id)

	if err := s.channels.Delete(string(id)); err != nil {
		return err
	}
	return s.deleteHTTPMeta(id)
}

// HTTPChannelScope returns an HTTP channel's metadata without needing the channel to be active.
func (s *Service) HTTPChannelScope(id ChannelID) (HTTPChannel, error) {
	return s.loadHTTPMeta(id)
}

// ListHTTPChannels reads every HTTP channel's metadata off disk, since httpMetaDir is the
// source of truth rather than an in-memory index, newest first.
func (s *Service) ListHTTPChannels() ([]HTTPChannel, error) {
	entries, err := os.ReadDir(s.httpMetaDir)
	if err != nil {
		return nil, fmt.Errorf("vault: reading %s: %w", s.httpMetaDir, err)
	}
	channels := make([]HTTPChannel, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := ChannelID(e.Name()[:len(e.Name())-len(".json")])
		c, err := s.loadHTTPMeta(id)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	slices.SortFunc(channels, func(a, b HTTPChannel) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return channels, nil
}

func (s *Service) saveHTTPMeta(c HTTPChannel) error {
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("vault: encoding metadata for channel %s: %w", c.ID, err)
	}
	return os.WriteFile(s.httpMetaPath(c.ID), b, 0o600)
}

func (s *Service) loadHTTPMeta(id ChannelID) (HTTPChannel, error) {
	b, err := os.ReadFile(s.httpMetaPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return HTTPChannel{}, ErrNotFound
	}
	if err != nil {
		return HTTPChannel{}, fmt.Errorf("vault: reading metadata for channel %s: %w", id, err)
	}
	var c HTTPChannel
	if err := json.Unmarshal(b, &c); err != nil {
		return HTTPChannel{}, fmt.Errorf("vault: decoding metadata for channel %s: %w", id, err)
	}
	return c, nil
}

func (s *Service) deleteHTTPMeta(id ChannelID) error {
	err := os.Remove(s.httpMetaPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Service) httpMetaPath(id ChannelID) string {
	return filepath.Join(s.httpMetaDir, string(id)+".json")
}

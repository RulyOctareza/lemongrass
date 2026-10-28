// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package agent maps short, model-facing channel ids to the vault's real channel ids and forwards query requests to the vault, holding no decryption power of its own.
package agent

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/faizalv/lemongrass/internal/dbgate"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

// ErrNoSuchChannel is the same for a mistyped id and one lost to a daemon restart, since the agent
// keeps no record of past ids, so its text covers both.
var ErrNoSuchChannel = errors.New("agent: no such channel, or it has expired. Ask the human for a fresh channel id.")

// Service holds a gatekeeper.Client and the short-id-to-real-channel-id mapping, never a root secret or credential material.
type Service struct {
	vaultClient *gatekeeper.Client

	mu   sync.Mutex
	byID map[string]vault.ChannelID
}

func NewService(vaultClient *gatekeeper.Client) *Service {
	return &Service{
		vaultClient: vaultClient,
		byID:        make(map[string]vault.ChannelID),
	}
}

// RegisterChannel confirms realID exists at the vault, as either a db or an HTTP channel, and
// mints a short model-facing id mapped to it, taking no root secret of its own.
func (s *Service) RegisterChannel(realID vault.ChannelID) (string, error) {
	if _, err := s.vaultClient.ChannelScope(realID); err != nil {
		if _, httpErr := s.vaultClient.HTTPChannelScope(realID); httpErr != nil {
			return "", fmt.Errorf("agent: registering channel %s: %w", realID, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		id, err := newShortID()
		if err != nil {
			return "", err
		}
		if _, exists := s.byID[id]; exists {
			continue
		}
		s.byID[id] = realID
		return id, nil
	}
}

// Query resolves shortID to its real vault channel and forwards the query, returning the same error for an unregistered, forgotten, or mistyped id.
func (s *Service) Query(shortID string, sqlText string) (dbgate.QueryResult, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return dbgate.QueryResult{}, ErrNoSuchChannel
	}
	result, err := s.vaultClient.Query(realID, sqlText)
	return result, redactID(err, realID, shortID)
}

// Execute resolves shortID to its real vault channel and forwards the write, returning the same
// error for an unregistered, forgotten, or mistyped id. actor is the caller-facing identity the
// call is audited under, forwarded as-is to the vault.
func (s *Service) Execute(shortID string, sqlText string, commit bool, actor string) (dbgate.WriteResult, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return dbgate.WriteResult{}, ErrNoSuchChannel
	}
	result, err := s.vaultClient.Execute(realID, sqlText, commit, actor)
	return result, redactID(err, realID, shortID)
}

// RequestHTTP resolves shortID to its real vault channel and forwards the HTTP request,
// returning the same error for an unregistered, forgotten, or mistyped id. actor is the
// caller-facing identity the call is audited under, forwarded as-is to the vault.
func (s *Service) RequestHTTP(shortID, user, method, path string, body []byte, contentType string, headers []vault.Header, actor string) (restergate.HTTPResult, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return restergate.HTTPResult{}, ErrNoSuchChannel
	}
	result, err := s.vaultClient.RequestHTTP(realID, user, method, path, body, contentType, headers, actor)
	return result, redactID(err, realID, shortID)
}

// RequestHTTPDownload resolves shortID to its real vault channel and forwards the download request, returning the same error for an unregistered, forgotten, or mistyped id. A streamed result's body is open and the caller must close it.
func (s *Service) RequestHTTPDownload(shortID, user, method, path string, body []byte, contentType string, headers []vault.Header) (gatekeeper.DownloadResult, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return gatekeeper.DownloadResult{}, ErrNoSuchChannel
	}
	res, err := s.vaultClient.RequestHTTPDownload(realID, user, method, path, body, contentType, headers)
	return res, redactID(err, realID, shortID)
}

// HTTPChannelInfo resolves shortID to its real vault channel and returns its model-facing
// summary, returning the same error for an unregistered, forgotten, or mistyped id.
func (s *Service) HTTPChannelInfo(shortID string) (restergate.HTTPChannelInfo, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return restergate.HTTPChannelInfo{}, ErrNoSuchChannel
	}
	info, err := s.vaultClient.HTTPChannelInfo(realID)
	return info, redactID(err, realID, shortID)
}

// HTTPUsers resolves shortID to its real vault channel and lists its rester's users with their
// tags, returning the same error for an unregistered, forgotten, or mistyped id.
func (s *Service) HTTPUsers(shortID string) ([]restergate.HTTPUserInfo, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return nil, ErrNoSuchChannel
	}
	users, err := s.vaultClient.HTTPChannelUsers(realID)
	return users, redactID(err, realID, shortID)
}

// FlushHTTPTokens resolves shortID to its real vault channel and evicts its cached tokens for
// user, or for every user when user is empty, returning the same error for an unregistered,
// forgotten, or mistyped id.
func (s *Service) FlushHTTPTokens(shortID, user string) (int, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return 0, ErrNoSuchChannel
	}
	flushed, err := s.vaultClient.FlushHTTPTokens(realID, user)
	return flushed, redactID(err, realID, shortID)
}

// AppLogin resolves shortID to its real vault channel and logs its app in on the connected
// phone as user, returning the same error for an unregistered, forgotten, or mistyped id.
func (s *Service) AppLogin(shortID, user, serial string) (restergate.AppLoginResult, error) {
	realID, ok := s.lookup(shortID)
	if !ok {
		return restergate.AppLoginResult{}, ErrNoSuchChannel
	}
	res, err := s.vaultClient.AppLogin(realID, user, serial)
	return res, redactID(err, realID, shortID)
}

// redactID replaces the real channel id in a vault error with the caller's short id, since
// the real id is a credential the model must never see, including inside wrapped file errors.
func redactID(err error, realID vault.ChannelID, shortID string) error {
	if err == nil || !strings.Contains(err.Error(), string(realID)) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), string(realID), shortID))
}

func (s *Service) lookup(shortID string) (vault.ChannelID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byID[shortID]
	return id, ok
}

// Forget drops shortID's mapping and succeeds even if shortID was never registered.
func (s *Service) Forget(shortID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, shortID)
}

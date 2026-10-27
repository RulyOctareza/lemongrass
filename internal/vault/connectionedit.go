// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"errors"
	"fmt"
	"strings"
)

// GetConnection decrypts dbName's stored connection string in full, password included, for a
// human editing it. It is never reachable from the agent or the CLI.
func (s *Service) GetConnection(rootSecret, dbName string) (string, error) {
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return "", err
	}
	defer Zero(rootKey)

	b, err := s.creds.Get(dbName, rootKey)
	if err != nil {
		return "", fmt.Errorf("vault: reading credential for %s: %w", dbName, err)
	}
	defer Zero(b)
	return string(b), nil
}

// ReplaceConnection stores connString as dbName after check accepts the existing value, then
// re-wraps it for every db channel minted from that connection, active or not, and invalidates
// their cached state so the next query connects with the new value. The existing credential is
// decrypted first, which fails on a wrong passphrase or an unknown name before anything is
// written. A channel that cannot be rewritten is named in the returned error; saving the same
// edit again retries.
func (s *Service) ReplaceConnection(rootSecret, dbName, connString string, check func(old string) error) error {
	old, err := s.GetConnection(rootSecret, dbName)
	if err != nil {
		return err
	}
	if check != nil {
		if err := check(old); err != nil {
			return err
		}
	}

	plain := []byte(connString)
	defer Zero(plain)
	if err := s.PutCredential(rootSecret, dbName, plain); err != nil {
		return err
	}

	channels, err := s.ListChannels()
	if err != nil {
		return err
	}
	var failed []string
	for _, c := range channels {
		if c.DBName != dbName {
			continue
		}
		if err := s.rewrapChannel(rootSecret, c, plain); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", c.ID, err))
			continue
		}
		s.invalidate(c.ID)
	}
	if len(failed) > 0 {
		return errors.New("vault: connection " + dbName + " saved, but these channels still hold the old credential, save again to retry: " + strings.Join(failed, "; "))
	}
	return nil
}

func (s *Service) rewrapChannel(rootSecret string, c Channel, plain []byte) error {
	key, err := DeriveKey(rootSecret, c.Salt)
	if err != nil {
		return err
	}
	defer Zero(key)
	return s.channels.Put(string(c.ID), key, plain)
}

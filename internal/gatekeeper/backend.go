// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package gatekeeper serves the vault and its gates over the daemon's unix socket and dials that socket as a client.
package gatekeeper

import (
	"path/filepath"
	"time"

	"github.com/faizalv/lemongrass/internal/channelaudit"
	"github.com/faizalv/lemongrass/internal/dbgate"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

// Backend is everything the daemon serves: the vault for credential keeping and the gates that execute against it.
type Backend struct {
	*vault.Service
	DB   *dbgate.Gate
	HTTP *restergate.Gate
}

// NewBackend opens the vault under dir, attaches both gates to it, and opens the shared
// connector audit log in a sibling file so both gates can write to it.
func NewBackend(dir string) (*Backend, error) {
	svc, err := vault.NewService(dir)
	if err != nil {
		return nil, err
	}
	b := NewBackendFor(svc)
	// A project removed from the registry while the daemon was down loses its policy here. A failed prune leaves the stale entries, which match no project and do no harm.
	_ = b.pruneProjectPolicies()
	// A failed expiry leaves a channel keyless and unusable, only labelled open, so it does not stop the daemon.
	_ = svc.ExpireKeylessChannels(time.Now())

	audit, err := channelaudit.Open(filepath.Join(dir, "audit.db"))
	if err != nil {
		return nil, err
	}
	b.DB.Audit = audit
	b.HTTP.Audit = audit

	return b, nil
}

// NewBackendFor attaches both gates to an already-open vault.
func NewBackendFor(svc *vault.Service) *Backend {
	return &Backend{Service: svc, DB: dbgate.New(svc), HTTP: restergate.New(svc)}
}

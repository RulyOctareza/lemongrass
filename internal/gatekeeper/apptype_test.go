// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

// An app-login on an api channel is refused by the gate before any device is opened, which proves the op is wired through the socket without needing a phone.
func TestIPCAppLoginReachesTheGate(t *testing.T) {
	backend, err := NewBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	const rootSecret = "root-secret"
	if err := backend.SetPassphrase(rootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	domain := vaulttest.Domain{
		BaseURL:       "http://127.0.0.1:1",
		LoginEndpoint: "/login",
		TokenPath:     "access_token",
		Users:         []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := backend.PutRester(rootSecret, "shop", domain.Rester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	c, err := backend.CreateHTTPChannel(rootSecret, "api", "shop", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatalf("CreateHTTPChannel: %v", err)
	}

	sockPath := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	go Serve(backend, l, vault.NewFailureLimiter(1000, time.Minute, time.Hour))
	t.Cleanup(func() { l.Close() })
	client := &Client{SocketPath: sockPath}

	if _, err := client.AppLogin(c.ID, "alice", ""); err == nil || !strings.Contains(err.Error(), "does not take app-login") {
		t.Errorf("AppLogin on an api channel = %v, want the gate's refusal", err)
	}
}

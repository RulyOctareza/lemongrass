// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package gatekeeper

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func newSlowServer(delay time.Duration) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return httptest.NewServer(mux)
}

func TestIPCRequestHTTPOutlivesConnDeadline(t *testing.T) {
	srv := newSlowServer(800 * time.Millisecond)
	defer srv.Close()

	backend, err := NewBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	const rootSecret = "root-secret"
	if err := backend.SetPassphrase(rootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := backend.PutRester(rootSecret, "test-domain", domain.Rester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	c, err := backend.CreateHTTPChannel(rootSecret, "test-channel", "test-domain", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"POST"}}, nil, time.Hour)
	if err != nil {
		t.Fatalf("CreateHTTPChannel: %v", err)
	}

	oldDeadline := connDeadline
	connDeadline = 300 * time.Millisecond
	t.Cleanup(func() { connDeadline = oldDeadline })

	sockPath := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	go Serve(backend, l, vault.NewFailureLimiter(1000, time.Minute, time.Hour))
	t.Cleanup(func() { l.Close() })
	client := &Client{SocketPath: sockPath}

	result, err := client.RequestHTTP(c.ID, "alice", "POST", "/import", []byte("x"), "text/csv", nil, "tab-1")
	if err != nil || result.Status != 200 {
		t.Errorf("RequestHTTP = %+v, %v, want a call slower than connDeadline to complete", result, err)
	}
}

func newCountingLoginServer(logins *atomic.Int32) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		n := logins.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "n": n})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return httptest.NewServer(mux)
}

func newEditableDomain(baseURL string, users []vault.ResterUser) vaulttest.Domain {
	return vaulttest.Domain{
		BaseURL:         baseURL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           users,
	}
}

func twoLoginUsers() []vault.ResterUser {
	return []vault.ResterUser{
		{Name: "alice", Fields: map[string]string{"u": "alice"}},
		{Name: "bob", Fields: map[string]string{"u": "bob"}},
	}
}

func TestIPCFlushHTTPTokensRoundTrips(t *testing.T) {
	var logins atomic.Int32
	srv := newCountingLoginServer(&logins)
	defer srv.Close()
	client := startTestServer(t)

	domain := newEditableDomain(srv.URL, twoLoginUsers())
	if err := client.PutRester(testRootSecret, "staging", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	c, err := client.CreateHTTPChannel(testRootSecret, "ch", "staging", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, 3600e9)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RequestHTTP(c.ID, "alice", "GET", "/api/x", nil, "", nil, ""); err != nil {
		t.Fatal(err)
	}
	flushed, err := client.FlushHTTPTokens(c.ID, "")
	if err != nil || flushed != 1 {
		t.Errorf("FlushHTTPTokens = %d, %v, want 1", flushed, err)
	}
	if _, err := client.FlushHTTPTokens(c.ID, "mallory"); err == nil {
		t.Error("expected an error for an unknown user")
	}
}

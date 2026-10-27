// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func TestServiceFlushHTTPTokens(t *testing.T) {
	var logins atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		logins.Add(1)
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	vaultClient := startTestVault(t)
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := vaultClient.PutRester(testRootSecret, "staging", domain.Rester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	c, err := vaultClient.CreateHTTPChannel(testRootSecret, "test-channel", "staging", vault.DefaultEnvironment, vault.TypeAPI, fullHTTPScope(), nil, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateHTTPChannel: %v", err)
	}
	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := svc.RequestHTTP(shortID, "", "GET", "/api/x", nil, "", nil, ""); err != nil {
			t.Fatalf("RequestHTTP: %v", err)
		}
	}
	if logins.Load() != 1 {
		t.Fatalf("logins = %d, want 1 before the flush", logins.Load())
	}

	flushed, err := svc.FlushHTTPTokens(shortID, "")
	if err != nil || flushed != 1 {
		t.Fatalf("FlushHTTPTokens = %d, %v, want 1", flushed, err)
	}
	if _, err := svc.RequestHTTP(shortID, "", "GET", "/api/x", nil, "", nil, ""); err != nil {
		t.Fatalf("RequestHTTP after flush: %v", err)
	}
	if logins.Load() != 2 {
		t.Errorf("logins = %d, want 2 after the flush", logins.Load())
	}

	if _, err := svc.FlushHTTPTokens("BOGUS1", ""); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("FlushHTTPTokens with an unregistered short id: got %v, want ErrNoSuchChannel", err)
	}
}

func TestServiceRequestHTTPCarriesBinaryBodyAndContentType(t *testing.T) {
	var gotType string
	var gotBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	vaultClient := startTestVault(t)
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := vaultClient.PutRester(testRootSecret, "staging", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	c, err := vaultClient.CreateHTTPChannel(testRootSecret, "ch", "staging", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"POST"}}, nil, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatal(err)
	}

	want := append([]byte("PK\x03\x04"), 0, 1, 2, 0xff, 0xfe, '\r', '\n', 0)
	const xlsx = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if _, err := svc.RequestHTTP(shortID, "", "POST", "/import", want, xlsx, nil, ""); err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	if gotType != xlsx || !bytes.Equal(gotBody, want) {
		t.Errorf("upstream saw type %q, body identical = %v", gotType, bytes.Equal(gotBody, want))
	}
}

func TestIPCRequestHTTPOutlivesAgentConnDeadline(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(800 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	vaultClient := startTestVault(t)
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := vaultClient.PutRester(testRootSecret, "staging", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	c, err := vaultClient.CreateHTTPChannel(testRootSecret, "ch", "staging", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"POST"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	oldDeadline, oldRequest := connDeadline, restergate.RequestTimeout
	connDeadline, restergate.RequestTimeout = 300*time.Millisecond, 5*time.Second
	t.Cleanup(func() { connDeadline, restergate.RequestTimeout = oldDeadline, oldRequest })

	agentClient := startTestAgent(t, vaultClient)
	shortID, err := agentClient.RegisterChannel(c.ID)
	if err != nil {
		t.Fatal(err)
	}

	result, err := agentClient.RequestHTTP(shortID, "alice", "POST", "/import", []byte("x"), "text/csv", nil, "tab-1")
	if err != nil || result.Status != 200 {
		t.Errorf("RequestHTTP = %+v, %v, want a call slower than the agent's connDeadline to complete", result, err)
	}
}

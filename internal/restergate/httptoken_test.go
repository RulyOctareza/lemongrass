// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package restergate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

// testSvc pairs a vault with the Gate attached to it, the way the daemon does.
type testSvc struct {
	*vault.Service
	*Gate
	t   *testing.T
	dir string
}

func newTestService(t *testing.T) *testSvc {
	t.Helper()
	dir := t.TempDir()
	svc, err := vault.NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &testSvc{Service: svc, Gate: New(svc), t: t, dir: dir}
}

// forgetKey simulates a vault restart: the metadata and wrapped copies persist on disk, but no channel key is cached and the gate's tokens are gone.
func (s *testSvc) forgetKey(id vault.ChannelID) {
	s.t.Helper()
	svc, err := vault.NewService(s.dir)
	if err != nil {
		s.t.Fatalf("NewService after restart: %v", err)
	}
	s.Service, s.Gate = svc, New(svc)
}

func TestObtainTokenCachesUntilNearExpiry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	}))
	defer srv.Close()

	svc := newTestService(t)
	domain := vault.ResolvedRester{
		BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 3600,
		Users: []vault.ResterUser{{Name: "alice", Fields: map[string]string{"username": "alice"}}},
	}
	id, _ := vault.NewChannelID()

	token1, _, err := svc.obtainToken(id, domain, "alice")
	if err != nil {
		t.Fatalf("obtainToken: %v", err)
	}
	token2, _, err := svc.obtainToken(id, domain, "alice")
	if err != nil {
		t.Fatalf("obtainToken (cached): %v", err)
	}
	if token1 != token2 || token1 != "tok" {
		t.Errorf("tokens = %q, %q, want both \"tok\"", token1, token2)
	}
	if calls != 1 {
		t.Errorf("login called %d times, want 1 (second call should hit the cache)", calls)
	}
}

func TestObtainTokenRefreshesNearExpiry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]string{"access_token": "fresh"})
	}))
	defer srv.Close()

	svc := newTestService(t)
	domain := vault.ResolvedRester{
		BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 3600,
		Users: []vault.ResterUser{{Name: "alice", Fields: map[string]string{"username": "alice"}}},
	}
	id, _ := vault.NewChannelID()

	if err := svc.setCachedToken(id, "alice", "stale", time.Now().Add(5*time.Second)); err != nil {
		t.Fatalf("setCachedToken: %v", err)
	}

	token, _, err := svc.obtainToken(id, domain, "alice")
	if err != nil {
		t.Fatalf("obtainToken: %v", err)
	}
	if token != "fresh" {
		t.Errorf("token = %q, want \"fresh\" (should have refreshed, cached token was within the margin)", token)
	}
	if calls != 1 {
		t.Errorf("login called %d times, want 1", calls)
	}
}

func TestObtainTokenBYOTNeverCallsLogin(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer srv.Close()

	svc := newTestService(t)
	domain := vault.ResolvedRester{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "bot", Token: "pasted"}},
	}
	id, _ := vault.NewChannelID()

	token, _, err := svc.obtainToken(id, domain, "bot")
	if err != nil {
		t.Fatalf("obtainToken: %v", err)
	}
	if token != "pasted" {
		t.Errorf("token = %q, want \"pasted\"", token)
	}
	if calls != 0 {
		t.Errorf("login endpoint called %d times, want 0 for a bring-your-own-token user", calls)
	}
}

func TestMarkTokenUsedAndForgetTokens(t *testing.T) {
	svc := newTestService(t)
	id, _ := vault.NewChannelID()

	if err := svc.setCachedToken(id, "alice", "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("setCachedToken: %v", err)
	}
	if cached, ok := svc.cachedTokenFor(id, "alice"); !ok || cached.everUsed {
		t.Errorf("cachedTokenFor before markTokenUsed: ok=%v everUsed=%v, want ok=true everUsed=false", ok, cached.everUsed)
	}

	svc.markTokenUsed(id, "alice")
	if cached, ok := svc.cachedTokenFor(id, "alice"); !ok || !cached.everUsed {
		t.Errorf("cachedTokenFor after markTokenUsed: ok=%v everUsed=%v, want ok=true everUsed=true", ok, cached.everUsed)
	}

	svc.forgetTokens(id)
	if _, ok := svc.cachedTokenFor(id, "alice"); ok {
		t.Error("cachedTokenFor after forgetTokens: ok=true, want false")
	}
}

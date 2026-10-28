// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

type headerFixture struct {
	svc     *testSvc
	id      vault.ChannelID
	logins  []http.Header
	calls   []http.Header
	other   []http.Header
	revoked map[string]bool
}

// newHeaderFixture serves /login, /api/x, /api/stale (refuses a token after its first use) and /api/hop (redirects to another host) behind a domain that sets anthropic-version.
func newHeaderFixture(t *testing.T) *headerFixture {
	t.Helper()
	f := &headerFixture{revoked: map[string]bool{}}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.other = append(f.other, r.Header.Clone())
	}))
	t.Cleanup(other.Close)

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		f.logins = append(f.logins, r.Header.Clone())
		json.NewEncoder(w).Encode(map[string]string{"access_token": fmt.Sprintf("tok%d", len(f.logins))})
	})
	mux.HandleFunc("/api/x", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.Header.Clone())
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/stale", func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.Header.Clone())
		token := r.Header.Get("X-Api-Key")
		if f.revoked[token] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.revoked[token] = true
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/hop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/file", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	f.svc = newTestService(t)
	if err := f.svc.SetPassphrase("root-secret"); err != nil {
		t.Fatal(err)
	}
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "X-Api-Key"},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
		Headers:         []vault.Header{{Name: "anthropic-version", Value: "2023-06-01"}},
	}
	if err := f.svc.PutRester("root-secret", "d", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	c, err := f.svc.CreateHTTPChannel("root-secret", "ch", "d", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.id = c.ID
	return f
}

var betaHeader = []vault.Header{{Name: "anthropic-beta", Value: "files-api"}}

func checkBothHeaders(t *testing.T, h http.Header) {
	t.Helper()
	if h.Get("Anthropic-Version") != "2023-06-01" || h.Get("Anthropic-Beta") != "files-api" {
		t.Errorf("upstream headers = %v, want the domain's anthropic-version and the call's anthropic-beta", h)
	}
}

func TestDomainAndCallHeadersReachTheUpstream(t *testing.T) {
	f := newHeaderFixture(t)
	if _, err := f.svc.RequestHTTP(f.id, "alice", "GET", "/api/x", nil, "", betaHeader); err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("upstream saw %d calls, want 1", len(f.calls))
	}
	checkBothHeaders(t, f.calls[0])
	if f.calls[0].Get("X-Api-Key") != "tok1" {
		t.Errorf("token header = %q, want tok1", f.calls[0].Get("X-Api-Key"))
	}
	if len(f.logins) != 1 || f.logins[0].Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("login headers = %v, want the domain's anthropic-version on the login", f.logins)
	}
	if f.logins[0].Get("Anthropic-Beta") != "" {
		t.Errorf("login carries the call's header %q, want only the domain's", f.logins[0].Get("Anthropic-Beta"))
	}
}

func TestDownloadSendsBothHeaders(t *testing.T) {
	f := newHeaderFixture(t)
	dl, err := f.svc.DownloadHTTP(f.id, "alice", "GET", "/api/x", nil, "", betaHeader)
	if err != nil {
		t.Fatalf("DownloadHTTP: %v", err)
	}
	io.Copy(io.Discard, dl.Body)
	dl.Body.Close()
	checkBothHeaders(t, f.calls[0])
}

func TestRetriedCallResendsBothHeaders(t *testing.T) {
	f := newHeaderFixture(t)
	for i := range 2 {
		result, err := f.svc.RequestHTTP(f.id, "alice", "GET", "/api/stale", nil, "", betaHeader)
		if err != nil || result.Status != 200 {
			t.Fatalf("call %d: RequestHTTP = %v, %v, want 200", i+1, result.Status, err)
		}
	}
	if len(f.calls) != 3 {
		t.Fatalf("upstream saw %d calls, want 3 (one retry)", len(f.calls))
	}
	checkBothHeaders(t, f.calls[2])
	if len(f.logins) != 2 || f.logins[1].Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("re-login headers = %v, want the domain's anthropic-version", f.logins)
	}
}

func TestRefusedCallHeaders(t *testing.T) {
	cases := []struct {
		name    string
		headers []vault.Header
		want    string
	}{
		{"token header", []vault.Header{{Name: "x-api-key", Value: "mine"}}, `header "x-api-key" is set by the proxy`},
		{"authorization", []vault.Header{{Name: "Authorization", Value: "Bearer mine"}}, `header "Authorization" is set by the proxy`},
		{"cookie", []vault.Header{{Name: "cookie", Value: "a=b"}}, `header "cookie" is set by the proxy`},
		{"content type", []vault.Header{{Name: "Content-Type", Value: "text/plain"}}, `header "Content-Type" is set by the proxy`},
		{"domain header", []vault.Header{{Name: "ANTHROPIC-VERSION", Value: "2099-01-01"}}, `header "ANTHROPIC-VERSION" is set by the rester`},
		{"repeated", []vault.Header{{Name: "X-A", Value: "1"}, {Name: "x-a", Value: "2"}}, `header "x-a" is given more than once`},
		{"newline", []vault.Header{{Name: "X-A", Value: "1\r\nX-B: 2"}}, `header "X-A" has a value that is not a single line`},
	}
	f := newHeaderFixture(t)
	for _, c := range cases {
		_, err := f.svc.RequestHTTP(f.id, "alice", "GET", "/api/x", nil, "", c.headers)
		if err == nil || !strings.HasPrefix(err.Error(), "restergate: ") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: RequestHTTP = %v, want an error containing %q", c.name, err, c.want)
		}
		if strings.Contains(fmt.Sprint(err), "2023-06-01") {
			t.Errorf("%s: error %q echoes the domain header's value", c.name, err)
		}
	}
	if len(f.calls) != 0 || len(f.logins) != 0 {
		t.Errorf("upstream saw %d calls and %d logins, want none for refused headers", len(f.calls), len(f.logins))
	}
}

func TestCrossHostRedirectDropsDomainHeadersKeepsCallHeaders(t *testing.T) {
	f := newHeaderFixture(t)
	if _, err := f.svc.RequestHTTP(f.id, "alice", "GET", "/api/hop", nil, "", betaHeader); err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	if len(f.other) != 1 {
		t.Fatalf("other host saw %d requests, want 1", len(f.other))
	}
	got := f.other[0]
	if got.Get("Anthropic-Version") != "" || got.Get("X-Api-Key") != "" {
		t.Errorf("other host headers = %v, want the domain header and the token dropped", got)
	}
	if got.Get("Anthropic-Beta") != "files-api" {
		t.Errorf("other host anthropic-beta = %q, want the call's header kept", got.Get("Anthropic-Beta"))
	}
}

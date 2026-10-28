// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

type seenRequest struct {
	header http.Header
	query  string
}

func newRedirectFixture(t *testing.T, placement vault.TokenPlacement, redirectTo func(other *httptest.Server) string) (*testSvc, vault.ChannelID, *[]seenRequest) {
	t.Helper()
	var seen []seenRequest
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, seenRequest{header: r.Header.Clone(), query: r.URL.RawQuery})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"from": "other"})
	}))
	t.Cleanup(other.Close)

	mux := http.NewServeMux()
	var base *httptest.Server
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-secret"})
	})
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTo(other), http.StatusFound)
	})
	mux.HandleFunc("/local", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base.URL+"/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"auth": r.Header.Get(placement.Name), "query": r.URL.RawQuery})
	})
	base = httptest.NewServer(mux)
	t.Cleanup(base.Close)

	svc := newTestService(t)
	if err := svc.SetPassphrase("root-secret"); err != nil {
		t.Fatal(err)
	}
	domain := vaulttest.Domain{
		BaseURL:         base.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  placement,
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := svc.PutRester("root-secret", "d", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateHTTPChannel("root-secret", "ch", "d", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return svc, c.ID, &seen
}

func TestCrossHostRedirectDropsTheToken(t *testing.T) {
	placements := map[string]vault.TokenPlacement{
		"authorization header": {Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		"custom header":        {Kind: vault.PlacementHeader, Name: "X-Api-Key"},
		"cookie":               {Kind: vault.PlacementCookie, Name: "session"},
		"query":                {Kind: vault.PlacementQuery, Name: "access_token"},
	}
	for name, placement := range placements {
		t.Run(name, func(t *testing.T) {
			svc, id, seen := newRedirectFixture(t, placement, func(other *httptest.Server) string {
				return other.URL + "/file?X-Amz-Signature=presigned"
			})

			result, err := svc.RequestHTTP(id, "alice", "GET", "/download", nil, "", nil)
			if err != nil {
				t.Fatalf("RequestHTTP: %v", err)
			}
			if result.Status != 200 {
				t.Fatalf("Status = %d, want the redirected download to succeed", result.Status)
			}
			if len(*seen) != 1 {
				t.Fatalf("other host saw %d requests, want 1", len(*seen))
			}
			got := (*seen)[0]
			for h, values := range got.header {
				for _, v := range values {
					if strings.Contains(v, "tok-secret") {
						t.Errorf("header %s carries the token on the other host: %q", h, v)
					}
				}
			}
			if strings.Contains(got.query, "tok-secret") || strings.Contains(got.query, "access_token") {
				t.Errorf("query carries the token on the other host: %q", got.query)
			}
			if !strings.Contains(got.query, "X-Amz-Signature=presigned") {
				t.Errorf("query = %q, want the presigned signature kept", got.query)
			}
		})
	}
}

func TestSameHostRedirectKeepsTheToken(t *testing.T) {
	placement := vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "X-Api-Key"}
	svc, id, _ := newRedirectFixture(t, placement, func(*httptest.Server) string { return "" })

	result, err := svc.RequestHTTP(id, "alice", "GET", "/local", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	body, ok := result.Body.(map[string]any)
	if !ok || body["auth"] != "tok-secret" {
		t.Errorf("body = %v, want the token kept on a same-host redirect", result.Body)
	}
}

func TestLoginRedirectToAnotherHostIsRefused(t *testing.T) {
	var otherHits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherHits++ }))
	defer other.Close()
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer base.Close()

	domain := vault.ResolvedRester{BaseURL: base.URL, LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 60}
	user := vault.ResterUser{Name: "alice", Fields: map[string]string{"password": "s3cret-pw"}}
	if _, _, err := loginHTTP(domain, user); err == nil {
		t.Fatal("a login redirected to another host was followed")
	}
	if otherHits != 0 {
		t.Errorf("the other host received %d requests, want 0", otherHits)
	}
}

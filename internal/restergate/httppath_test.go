// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

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

func TestResolveRequestTarget(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		input     string
		wantScope string
		wantURL   string
		wantErr   string
	}{
		{name: "plain path", base: "https://api.example.com", input: "/orders/42", wantScope: "/orders/42", wantURL: "https://api.example.com/orders/42"},
		{name: "missing leading slash", base: "https://api.example.com", input: "orders/42", wantScope: "/orders/42", wantURL: "https://api.example.com/orders/42"},
		{name: "query kept", base: "https://api.example.com", input: "/orders?limit=5", wantScope: "/orders", wantURL: "https://api.example.com/orders?limit=5"},
		{name: "fragment dropped", base: "https://api.example.com", input: "/orders#top", wantScope: "/orders", wantURL: "https://api.example.com/orders"},
		{name: "trailing slash kept", base: "https://api.example.com", input: "/orders/", wantScope: "/orders/", wantURL: "https://api.example.com/orders/"},
		{name: "full url under base", base: "https://api.example.com", input: "https://api.example.com/orders/42?x=1", wantScope: "/orders/42", wantURL: "https://api.example.com/orders/42?x=1"},
		{name: "full url host case", base: "https://api.example.com", input: "https://API.example.com/orders", wantScope: "/orders", wantURL: "https://api.example.com/orders"},
		{name: "full url on another host", base: "https://api.example.com", input: "https://evil.example/orders", wantErr: "not under this rester's base URL"},
		{name: "full url wrong scheme", base: "https://api.example.com", input: "http://api.example.com/orders", wantErr: "not under this rester's base URL"},
		{name: "full url with userinfo", base: "https://api.example.com", input: "https://api.example.com@evil.example/orders", wantErr: "not under this rester's base URL"},
		{name: "userinfo shaped path stays a path", base: "https://api.example.com", input: "@evil.example/x", wantScope: "/@evil.example/x", wantURL: "https://api.example.com/@evil.example/x"},
		{name: "scheme relative", base: "https://api.example.com", input: "//evil.example/x", wantErr: "scheme-relative"},
		{name: "host and port shaped", base: "https://api.example.com", input: "localhost:8080/x", wantScope: "/localhost:8080/x", wantURL: "https://api.example.com/localhost:8080/x"},
		{name: "empty", base: "https://api.example.com", input: "  ", wantErr: "empty"},
		{name: "dot segments cleaned", base: "https://api.example.com", input: "/users/../users/1/change-password", wantScope: "/users/1/change-password", wantURL: "https://api.example.com/users/1/change-password"},
		{name: "dots above root", base: "https://api.example.com", input: "/../../orders", wantScope: "/orders", wantURL: "https://api.example.com/orders"},
		{name: "double slash collapsed", base: "https://api.example.com", input: "/users//1", wantScope: "/users/1", wantURL: "https://api.example.com/users/1"},
		{name: "encoded char decoded", base: "https://api.example.com", input: "/users/1/change%2Dpassword", wantScope: "/users/1/change-password", wantURL: "https://api.example.com/users/1/change-password"},
		{name: "encoded slash rejected", base: "https://api.example.com", input: "/users/1%2Fchange-password", wantErr: "encoded slash"},
		{name: "encoded backslash rejected", base: "https://api.example.com", input: "/users%5C1", wantErr: "encoded slash"},
		{name: "base path prepended", base: "https://api.example.com/api/v1", input: "/orders", wantScope: "/orders", wantURL: "https://api.example.com/api/v1/orders"},
		{name: "base path repeated in path", base: "https://api.example.com/api/v1", input: "/api/v1/orders", wantScope: "/orders", wantURL: "https://api.example.com/api/v1/orders"},
		{name: "base path repeated without slash", base: "https://api.example.com/api/v1", input: "api/v1/orders", wantScope: "/orders", wantURL: "https://api.example.com/api/v1/orders"},
		{name: "base path only", base: "https://api.example.com/api/v1/", input: "/api/v1", wantScope: "/", wantURL: "https://api.example.com/api/v1/"},
		{name: "similar prefix not stripped", base: "https://api.example.com/api", input: "/apiary/x", wantScope: "/apiary/x", wantURL: "https://api.example.com/api/apiary/x"},
		{name: "full url with base path", base: "https://api.example.com/api/v1", input: "https://api.example.com/api/v1/orders", wantScope: "/orders", wantURL: "https://api.example.com/api/v1/orders"},
		{name: "full url outside base path", base: "https://api.example.com/api/v1", input: "https://api.example.com/orders", wantErr: "outside this rester's base path"},
		{name: "trailing slash on base", base: "https://api.example.com/", input: "/orders", wantScope: "/orders", wantURL: "https://api.example.com/orders"},
		{name: "base with port", base: "http://127.0.0.1:8080", input: "http://127.0.0.1:8080/orders", wantScope: "/orders", wantURL: "http://127.0.0.1:8080/orders"},
		{name: "base with other port", base: "http://127.0.0.1:8080", input: "http://127.0.0.1:9090/orders", wantErr: "not under this rester's base URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRequestTarget(tt.base, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ScopePath != tt.wantScope {
				t.Errorf("ScopePath = %q, want %q", got.ScopePath, tt.wantScope)
			}
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
		})
	}
}

func TestResolveRequestTargetInvalidBase(t *testing.T) {
	if _, err := resolveRequestTarget("not a url", "/x"); err == nil {
		t.Error("expected an error for a base URL with no host")
	}
}

func newLoginMux(hits *[]string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		*hits = append(*hits, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	return mux
}

func TestRequestHTTPFullURLUnderBaseAndReportedURL(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	result, err := svc.RequestHTTP(id, "alice", "GET", srv.URL+"/api/orders?limit=2", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Status = %d, want 200", result.Status)
	}
	if want := srv.URL + "/api/orders?limit=2"; result.URL != want {
		t.Errorf("URL = %q, want %q", result.URL, want)
	}
	if len(hits) != 1 || hits[0] != "/api/orders?limit=2" {
		t.Errorf("server saw %v, want [/api/orders?limit=2]", hits)
	}
}

func TestRequestHTTPNeverReachesAnotherHost(t *testing.T) {
	var hits, otherHits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	other := httptest.NewServer(newLoginMux(&otherHits))
	defer other.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	otherHost := strings.TrimPrefix(other.URL, "http://")
	for _, p := range []string{
		other.URL + "/steal",
		"//" + otherHost + "/steal",
		"@" + otherHost + "/steal",
	} {
		result, err := svc.RequestHTTP(id, "alice", "GET", p, nil, "", nil)
		if err == nil && !strings.HasPrefix(result.URL, srv.URL+"/") {
			t.Errorf("path %q was sent to %q", p, result.URL)
		}
	}
	if len(otherHits) != 0 {
		t.Errorf("the other host was reached: %v", otherHits)
	}
}

func TestRequestHTTPExclusionHoldsAgainstBypassShapes(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()

	svc := newTestService(t)
	const rootSecret = "root-secret"
	if err := svc.SetPassphrase(rootSecret); err != nil {
		t.Fatal(err)
	}
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := svc.PutRester(rootSecret, "d", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	scope := vault.HTTPScope{
		Methods:    []string{"PUT"},
		Exclusions: []vault.MethodPath{{Method: "PUT", PathPattern: "/users/*/change-password"}},
	}
	c, err := svc.CreateHTTPChannel(rootSecret, "ch", "d", vault.DefaultEnvironment, vault.TypeAPI, scope, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		"/users/1/change-password",
		"/users/1/change-password?x=1",
		"/users/../users/1/change-password",
		"/users/1/change%2Dpassword",
		"/users//1/change-password",
		srv.URL + "/users/1/change-password",
	} {
		if _, err := svc.RequestHTTP(c.ID, "alice", "PUT", p, nil, "", nil); err == nil || !strings.Contains(err.Error(), "does not grant") {
			t.Errorf("path %q: err = %v, want a scope denial", p, err)
		}
	}
	if len(hits) != 0 {
		t.Errorf("an excluded endpoint was reached: %v", hits)
	}

	if _, err := svc.RequestHTTP(c.ID, "alice", "PUT", "/users/1/profile", nil, "", nil); err != nil {
		t.Errorf("allowed path rejected: %v", err)
	}
}

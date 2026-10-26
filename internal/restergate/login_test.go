// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package restergate

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestJSONPathStringFlatAndNested(t *testing.T) {
	body := []byte(`{"access_token":"flat-token","data":{"token":"nested-token"},"expires_in":3600}`)

	if got, err := jsonPathString(body, "access_token"); err != nil || got != "flat-token" {
		t.Errorf("jsonPathString(flat) = %q, %v, want \"flat-token\", nil", got, err)
	}
	if got, err := jsonPathString(body, "data.token"); err != nil || got != "nested-token" {
		t.Errorf("jsonPathString(nested) = %q, %v, want \"nested-token\", nil", got, err)
	}
	if got, err := jsonPathString(body, "expires_in"); err != nil || got != "3600" {
		t.Errorf("jsonPathString(numeric) = %q, %v, want \"3600\", nil", got, err)
	}
	if _, err := jsonPathString(body, "missing"); err == nil {
		t.Error("jsonPathString(missing key) = nil error, want an error")
	}
}

func makeJWT(t *testing.T, exp int64) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":` + strconv.FormatInt(exp, 10) + `}`))
	return header + "." + payload + ".sig"
}

func TestJWTExpiry(t *testing.T) {
	want := time.Now().Add(time.Hour).Unix()
	token := makeJWT(t, want)

	got, err := jwtExpiry(token)
	if err != nil {
		t.Fatalf("jwtExpiry: %v", err)
	}
	if got.Unix() != want {
		t.Errorf("jwtExpiry() = %v, want unix %d", got, want)
	}

	if _, err := jwtExpiry("not-a-jwt"); err == nil {
		t.Error("jwtExpiry(malformed) = nil error, want an error")
	}
}

func TestByotExpiryFixedTTL(t *testing.T) {
	domain := vault.ResolvedRester{FixedTTLSeconds: 60}
	user := vault.ResterUser{Token: "pasted-token"}

	got, err := byotExpiry(domain, user)
	if err != nil {
		t.Fatalf("byotExpiry: %v", err)
	}
	if time.Until(got) > 61*time.Second || time.Until(got) < 59*time.Second {
		t.Errorf("byotExpiry() = %v, want ~60s from now", got)
	}
}

func TestByotExpiryJSONPathOriginRejected(t *testing.T) {
	domain := vault.ResolvedRester{TTLOrigin: "expires_in"}
	user := vault.ResterUser{Token: "pasted-token"}

	if _, err := byotExpiry(domain, user); err == nil {
		t.Error("byotExpiry with a JSON-path TTLOrigin for a BYOT user = nil error, want an error (no login response to read)")
	}
}

func TestLoginHTTPExtractsTokenAndFixedTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "alice" {
			http.Error(w, "bad creds", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "abc123"})
	}))
	defer srv.Close()

	domain := vault.ResolvedRester{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 30}
	user := vault.ResterUser{Fields: map[string]string{"username": "alice"}}

	token, expiresAt, err := loginHTTP(domain, user)
	if err != nil {
		t.Fatalf("loginHTTP: %v", err)
	}
	if token != "abc123" {
		t.Errorf("token = %q, want \"abc123\"", token)
	}
	if time.Until(expiresAt) > 31*time.Second || time.Until(expiresAt) < 29*time.Second {
		t.Errorf("expiresAt = %v, want ~30s from now", expiresAt)
	}
}

func TestLoginHTTPBadCredentialsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad creds", http.StatusUnauthorized)
	}))
	defer srv.Close()

	domain := vault.ResolvedRester{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token"}
	if _, _, err := loginHTTP(domain, vault.ResterUser{}); err == nil {
		t.Error("loginHTTP with a 401 response = nil error, want an error")
	}
}

func TestLoginHTTPSendsTypedFields(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(map[string]string{"access_token": "abc123"})
	}))
	defer srv.Close()

	domain := vault.ResolvedRester{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 30}
	user := vault.ResterUser{
		Fields:     map[string]string{"username": "alice", "site_id": "3", "remember": "false"},
		FieldTypes: map[string]string{"site_id": vault.FieldNumber, "remember": vault.FieldBoolean},
	}
	if _, _, err := loginHTTP(domain, user); err != nil {
		t.Fatalf("loginHTTP: %v", err)
	}
	if got["username"] != "alice" || got["site_id"] != float64(3) || got["remember"] != false {
		t.Errorf("login body = %#v, want username a string, site_id a number, remember a boolean", got)
	}
}

func TestLoginHTTPStatusErrorCarriesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"email is required"}`))
	}))
	defer srv.Close()

	domain := vault.ResolvedRester{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token"}
	_, _, err := loginHTTP(domain, vault.ResterUser{})

	var statusErr *LoginStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("loginHTTP with a 400 response = %v, want a *LoginStatusError", err)
	}
	if statusErr.Status != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", statusErr.Status)
	}
	if string(statusErr.Body) != `{"message":"email is required"}` {
		t.Errorf("Body = %q, want the response body", statusErr.Body)
	}
	if err.Error() != "restergate: login returned status 400" {
		t.Errorf("Error() = %q, want the status only", err.Error())
	}
}

func TestTestResterLoginBYOT(t *testing.T) {
	g := New(newTestService(t).Service)
	api := vault.ResterAPI{TTLOrigin: "jwt-exp"}
	env := vault.Environment{Name: "local", BaseURL: "http://localhost"}
	user := vault.ResterUser{Token: makeJWT(t, time.Now().Add(time.Hour).Unix())}
	if err := g.TestResterLogin(api, env, user); err != nil {
		t.Errorf("TestResterLogin(BYOT, valid jwt) = %v, want nil", err)
	}
	if err := g.TestResterLogin(api, env, vault.ResterUser{Token: "not-a-jwt"}); err == nil {
		t.Error("TestResterLogin(BYOT, invalid token) = nil, want an error")
	}
}

func TestTestResterLoginUsesTheEnvironmentsAddressAndHeaders(t *testing.T) {
	var gotHeader, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader, gotPath = r.Header.Get("X-Tenant"), r.URL.Path
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	}))
	defer srv.Close()
	api := vault.ResterAPI{LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 30, Headers: []vault.Header{{Name: "X-Tenant", Value: "shared"}}}
	env := vault.Environment{Name: "staging", BaseURL: srv.URL, Headers: []vault.Header{{Name: "x-tenant", Value: "staging"}}}
	g := New(newTestService(t).Service)
	if err := g.TestResterLogin(api, env, vault.ResterUser{Fields: map[string]string{"u": "alice"}}); err != nil {
		t.Fatalf("TestResterLogin: %v", err)
	}
	if gotPath != "/login" || gotHeader != "staging" {
		t.Errorf("login hit %q with X-Tenant %q, want /login with the environment's header", gotPath, gotHeader)
	}
}

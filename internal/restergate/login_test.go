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
	domain := vault.Domain{FixedTTLSeconds: 60}
	user := vault.DomainUser{Token: "pasted-token"}

	got, err := byotExpiry(domain, user)
	if err != nil {
		t.Fatalf("byotExpiry: %v", err)
	}
	if time.Until(got) > 61*time.Second || time.Until(got) < 59*time.Second {
		t.Errorf("byotExpiry() = %v, want ~60s from now", got)
	}
}

func TestByotExpiryJSONPathOriginRejected(t *testing.T) {
	domain := vault.Domain{TTLOrigin: "expires_in"}
	user := vault.DomainUser{Token: "pasted-token"}

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

	domain := vault.Domain{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 30}
	user := vault.DomainUser{Fields: map[string]string{"username": "alice"}}

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

	domain := vault.Domain{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token"}
	if _, _, err := loginHTTP(domain, vault.DomainUser{}); err == nil {
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

	domain := vault.Domain{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token", FixedTTLSeconds: 30}
	user := vault.DomainUser{
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

	domain := vault.Domain{BaseURL: srv.URL, LoginEndpoint: "/login", TokenPath: "access_token"}
	_, _, err := loginHTTP(domain, vault.DomainUser{})

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

func TestTestDomainLoginBYOT(t *testing.T) {
	g := &Gate{}
	domain := vault.Domain{TTLOrigin: "jwt-exp"}
	user := vault.DomainUser{Token: makeJWT(t, time.Now().Add(time.Hour).Unix())}

	if err := g.TestDomainLogin(domain, user); err != nil {
		t.Errorf("TestDomainLogin(BYOT, valid jwt) = %v, want nil", err)
	}

	if err := g.TestDomainLogin(domain, vault.DomainUser{Token: "not-a-jwt"}); err == nil {
		t.Error("TestDomainLogin(BYOT, invalid token) = nil, want an error")
	}
}

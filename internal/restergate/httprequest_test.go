package restergate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

// setUpHTTPChannel creates a domain and an active HTTP channel against it, returning the
// service, the channel id, and the server for the caller to add handlers to before use.
func setUpHTTPChannel(t *testing.T, srv *httptest.Server, users []vault.DomainUser, methods []string) (*testSvc, vault.ChannelID) {
	t.Helper()
	svc := newTestService(t)
	const rootSecret = "root-secret"
	if err := svc.SetPassphrase(rootSecret); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	domain := vault.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           users,
	}
	if err := svc.PutDomain(rootSecret, "test-domain", domain); err != nil {
		t.Fatalf("PutDomain: %v", err)
	}
	c, err := svc.CreateHTTPChannel(rootSecret, "test-channel", "test-domain", vault.HTTPScope{Methods: methods}, time.Hour)
	if err != nil {
		t.Fatalf("CreateHTTPChannel: %v", err)
	}
	return svc, c.ID
}

func TestRequestHTTPSuccessfulCall(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"orders": []int{1, 2, 3}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	result, err := svc.RequestHTTP(id, "alice", "GET", "/api/orders", nil, "", nil)
	if err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Status = %d, want 200", result.Status)
	}
	body, ok := result.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body = %T, want map[string]any", result.Body)
	}
	if _, ok := body["orders"]; !ok {
		t.Errorf("Body = %v, want an \"orders\" key", body)
	}
}

func TestRequestHTTPScopeDenied(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	if _, err := svc.RequestHTTP(id, "alice", "DELETE", "/api/orders/1", nil, "", nil); err == nil {
		t.Error("RequestHTTP with an ungranted method = nil error, want an error")
	}
}

func TestRequestHTTPExpiredChannelDenied(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	if _, err := svc.ActivateHTTP("root-secret", id, -time.Minute); err != nil {
		t.Fatalf("ActivateHTTP: %v", err)
	}

	if _, err := svc.RequestHTTP(id, "alice", "GET", "/api/orders", nil, "", nil); err == nil {
		t.Error("RequestHTTP on an expired channel = nil error, want an error")
	}
}

func TestRequestHTTPReactiveRetryOnStaleToken(t *testing.T) {
	loginCalls := 0
	revoked := map[string]bool{}

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		loginCalls++
		json.NewEncoder(w).Encode(map[string]string{"access_token": fmt.Sprintf("tok%d", loginCalls)})
	})
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")[len("Bearer "):]
		if revoked[token] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		revoked[token] = true // simulate the server invalidating a token right after its first successful use
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	if _, err := svc.RequestHTTP(id, "alice", "GET", "/api/orders", nil, "", nil); err != nil {
		t.Fatalf("first RequestHTTP: %v", err)
	}
	if loginCalls != 1 {
		t.Fatalf("loginCalls after first request = %d, want 1", loginCalls)
	}

	result, err := svc.RequestHTTP(id, "alice", "GET", "/api/orders", nil, "", nil)
	if err != nil {
		t.Fatalf("second RequestHTTP (should transparently retry after a stale-token 401): %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Status = %d, want 200 after the reactive retry", result.Status)
	}
	if loginCalls != 2 {
		t.Errorf("loginCalls after second request = %d, want 2 (one re-login for the retry)", loginCalls)
	}
}

func TestRequestHTTPBYOTStaleTokenIsNotRetried(t *testing.T) {
	revoked := map[string]bool{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")[len("Bearer "):]
		if revoked[token] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		revoked[token] = true
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "bot", Token: "pasted-token"}}, []string{"GET"})

	if _, err := svc.RequestHTTP(id, "bot", "GET", "/api/orders", nil, "", nil); err != nil {
		t.Fatalf("first RequestHTTP: %v", err)
	}

	if _, err := svc.RequestHTTP(id, "bot", "GET", "/api/orders", nil, "", nil); err == nil {
		t.Error("second RequestHTTP for a bring-your-own-token user's now-stale token = nil error, want an error (no login to retry with)")
	}
	if _, ok := svc.cachedTokenFor(id, "bot"); ok {
		t.Error("stale bring-your-own-token should be evicted from the cache after the failed retry attempt")
	}
}

func TestRequestHTTPSoleUserDefault(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.DomainUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	if _, err := svc.RequestHTTP(id, "", "GET", "/api/orders", nil, "", nil); err != nil {
		t.Fatalf("RequestHTTP with no user on a one-user domain: %v", err)
	}
}

func TestRequestHTTPUserErrorsListUsers(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	users := []vault.DomainUser{
		{Name: "alice", Fields: map[string]string{"u": "alice"}},
		{Name: "bob", Fields: map[string]string{"u": "bob"}},
	}
	svc, id := setUpHTTPChannel(t, srv, users, []string{"GET"})

	if _, err := svc.RequestHTTP(id, "", "GET", "/api/orders", nil, "", nil); err == nil || !strings.Contains(err.Error(), "alice, bob") {
		t.Errorf("missing user: err = %v, want one listing alice, bob", err)
	}
	if _, err := svc.RequestHTTP(id, "carol", "GET", "/api/orders", nil, "", nil); err == nil || !strings.Contains(err.Error(), `no user "carol"`) || !strings.Contains(err.Error(), "alice, bob") {
		t.Errorf("unknown user: err = %v, want one naming carol and listing alice, bob", err)
	}
	if len(hits) != 0 {
		t.Errorf("a request was sent despite the user error: %v", hits)
	}
}

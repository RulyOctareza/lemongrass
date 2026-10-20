package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func fullHTTPScope() vault.HTTPScope {
	return vault.HTTPScope{Methods: []string{"GET"}}
}

func TestServiceRegisterThenRequestHTTP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
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
	if len(shortID) != shortIDLength {
		t.Errorf("RegisterChannel returned id of length %d, want %d", len(shortID), shortIDLength)
	}

	result, err := svc.RequestHTTP(shortID, "alice", "GET", "/api/orders", nil, "", nil, "tab-1")
	if err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}
	if result.Status != 200 {
		t.Errorf("Status = %d, want 200", result.Status)
	}
}

func TestServiceRequestHTTPUnknownShortIDReturnsErrNoSuchChannel(t *testing.T) {
	svc := NewService(startTestVault(t))
	if _, err := svc.RequestHTTP("BOGUS1", "alice", "GET", "/api/orders", nil, "", nil, ""); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("RequestHTTP with an unregistered short id: got %v, want ErrNoSuchChannel", err)
	}
}

func TestServiceHTTPChannelInfo(t *testing.T) {
	srv := httptest.NewServer(http.NewServeMux())
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

	info, err := svc.HTTPChannelInfo(shortID)
	if err != nil {
		t.Fatalf("HTTPChannelInfo: %v", err)
	}
	if info.BaseURL != srv.URL || info.Status != restergate.HTTPStatusActive || len(info.Users) != 1 {
		t.Errorf("info = %+v, want an active channel on %s with one user", info, srv.URL)
	}

	if _, err := svc.HTTPChannelInfo("BOGUS1"); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("HTTPChannelInfo with an unregistered short id: got %v, want ErrNoSuchChannel", err)
	}
}

func TestServiceRequestHTTPFullURLAndDefaultUser(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
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

	result, err := svc.RequestHTTP(shortID, "", "GET", srv.URL+"/api/orders", nil, "", nil, "")
	if err != nil {
		t.Fatalf("RequestHTTP with a full URL and no user: %v", err)
	}
	if result.Status != 200 || result.URL != srv.URL+"/api/orders" {
		t.Errorf("result = status %d url %q, want 200 and %q", result.Status, result.URL, srv.URL+"/api/orders")
	}

	if _, err := svc.RequestHTTP(shortID, "", "GET", "https://elsewhere.example/api/orders", nil, "", nil, ""); err == nil {
		t.Error("a full URL on another host was accepted")
	}
}

func TestServiceHTTPUsers(t *testing.T) {
	vaultClient := startTestVault(t)
	domain := vaulttest.Domain{
		BaseURL:         "http://127.0.0.1:1",
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "employee", Fields: map[string]string{"password": "pw"}, Tags: []string{"tenant x"}}},
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

	users, err := svc.HTTPUsers(shortID)
	if err != nil {
		t.Fatalf("HTTPUsers: %v", err)
	}
	if len(users) != 1 || users[0].Name != "employee" || users[0].Tags[0] != "tenant x" {
		t.Errorf("users = %+v, want employee tagged tenant x", users)
	}

	if _, err := svc.HTTPUsers("BOGUS1"); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("HTTPUsers with an unregistered short id: got %v, want ErrNoSuchChannel", err)
	}
}

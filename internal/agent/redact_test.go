package agent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func TestHTTPErrorsNeverCarryTheRealChannelID(t *testing.T) {
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
		t.Fatal(err)
	}
	c, err := vaultClient.CreateHTTPChannel(testRootSecret, "ch", "staging", vault.DefaultEnvironment, vault.TypeAPI, fullHTTPScope(), nil, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	real := string(c.ID)

	_, err = svc.RequestHTTP(shortID, "alice", "POST", "/x", nil, "", nil, "")
	if err == nil || !strings.Contains(err.Error(), "does not grant") || strings.Contains(err.Error(), real) {
		t.Errorf("scope denial: err = %v, want a denial without the real id", err)
	}

	if err := vaultClient.RevokeHTTP(c.ID); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func() error{
		"request": func() error { _, err := svc.RequestHTTP(shortID, "alice", "GET", "/x", nil, "", nil, ""); return err },
		"info":    func() error { _, err := svc.HTTPChannelInfo(shortID); return err },
		"users":   func() error { _, err := svc.HTTPUsers(shortID); return err },
	} {
		err := call()
		if err == nil {
			t.Errorf("%s on a revoked channel returned no error", name)
			continue
		}
		if strings.Contains(err.Error(), real) {
			t.Errorf("%s error carries the real channel id: %v", name, err)
		}
	}
}

func TestQueryErrorsNeverCarryTheRealChannelID(t *testing.T) {
	vaultClient := startTestVault(t)
	if err := vaultClient.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatal(err)
	}
	c, err := vaultClient.CreateChannel(testRootSecret, "ch", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(vaultClient)
	shortID, err := svc.RegisterChannel(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := vaultClient.Revoke(c.ID); err != nil {
		t.Fatal(err)
	}

	_, err = svc.Query(shortID, "SELECT 1")
	if err == nil {
		t.Fatal("Query on a revoked channel returned no error")
	}
	if strings.Contains(err.Error(), string(c.ID)) {
		t.Errorf("Query error carries the real channel id: %v", err)
	}
}

func TestRedactID(t *testing.T) {
	real := vault.ChannelID("REALIDREALIDREALID")
	err := errors.New("vault: reading metadata for channel REALIDREALIDREALID: open /home/u/.lemongrass/vault/http-channels/REALIDREALIDREALID.json: no such file")

	got := redactID(err, real, "SHORT1")
	if strings.Contains(got.Error(), string(real)) || strings.Count(got.Error(), "SHORT1") != 2 {
		t.Errorf("redactID = %v, want both occurrences replaced by the short id", got)
	}
	if redactID(nil, real, "SHORT1") != nil {
		t.Error("redactID of nil is not nil")
	}
	plain := errors.New("vault: this channel has expired")
	if redactID(plain, real, "SHORT1") != plain {
		t.Error("an error without the id must pass through unchanged")
	}
}

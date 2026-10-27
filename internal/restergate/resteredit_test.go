// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package restergate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func TestHTTPChannelUsersListsNamesAndTagsOnly(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	users := []vault.ResterUser{
		{Name: "employee", Fields: map[string]string{"password": "s3cret-pw"}, Tags: []string{"tenant x", "low level"}},
		{Name: "admin", Token: "s3cret-token"},
	}
	svc, id := setUpHTTPChannel(t, srv, users, []string{"GET"})

	got, err := svc.HTTPChannelUsers(id)
	if err != nil {
		t.Fatalf("HTTPChannelUsers: %v", err)
	}
	if len(got) != 2 || got[0].Name != "employee" || len(got[0].Tags) != 2 || got[1].Name != "admin" || got[1].Tags == nil {
		t.Errorf("users = %+v", got)
	}
	raw, _ := json.Marshal(got)
	for _, secret := range []string{"s3cret-pw", "s3cret-token", "password"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("users JSON leaks %q: %s", secret, raw)
		}
	}

	svc.forgetKey(id)
	if _, err := svc.HTTPChannelUsers(id); err == nil {
		t.Error("expected an error listing users of an inactive channel")
	}
}

func TestUserErrorsIncludeTags(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	users := []vault.ResterUser{
		{Name: "employee", Fields: map[string]string{"u": "e"}, Tags: []string{"tenant x", "low level"}},
		{Name: "admin", Fields: map[string]string{"u": "a"}},
	}
	svc, id := setUpHTTPChannel(t, srv, users, []string{"GET"})

	_, err := svc.RequestHTTP(id, "", "GET", "/api/orders", nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), "employee (tenant x, low level)") || !strings.Contains(err.Error(), "admin") {
		t.Errorf("err = %v, want one listing employee with its tags and admin", err)
	}
}

func newEditableDomain(baseURL string, users []vault.ResterUser) vaulttest.Domain {
	return vaulttest.Domain{
		BaseURL:         baseURL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           users,
	}
}

func TestUpdateResterPropagatesToActiveAndInactiveChannels(t *testing.T) {
	var oldHits, newHits []string
	oldSrv := httptest.NewServer(newLoginMux(&oldHits))
	defer oldSrv.Close()
	newSrv := httptest.NewServer(newLoginMux(&newHits))
	defer newSrv.Close()

	svc, activeID := setUpHTTPChannel(t, oldSrv, []vault.ResterUser{
		{Name: "alice", Fields: map[string]string{"u": "alice"}},
		{Name: "bob", Fields: map[string]string{"u": "bob"}},
	}, []string{"GET"})
	// A second vault over the same directory creates the channel, so it exists on disk but svc holds no key for it.
	other, err := vault.NewService(svc.dir)
	if err != nil {
		t.Fatal(err)
	}
	inactive, err := other.CreateHTTPChannel("root-secret", "second", "test-rester", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, 3600e9)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RequestHTTP(activeID, "alice", "GET", "/api/x", nil, "", nil); err != nil {
		t.Fatalf("baseline request: %v", err)
	}
	if len(oldHits) != 1 {
		t.Fatalf("oldHits = %v, want one baseline hit", oldHits)
	}

	edited := newEditableDomain(newSrv.URL, []vault.ResterUser{
		{Name: "alice", Fields: map[string]string{"u": "alice2"}, Tags: []string{"renamed"}},
	})
	if err := svc.UpdateRester("root-secret", "test-rester", edited.Rester()); err != nil {
		t.Fatalf("UpdateRester: %v", err)
	}

	if _, err := svc.RequestHTTP(activeID, "alice", "GET", "/api/x", nil, "", nil); err != nil {
		t.Fatalf("request on the active channel after the edit: %v", err)
	}
	if len(newHits) != 1 || len(oldHits) != 1 {
		t.Errorf("newHits = %v, oldHits = %v, want the request to reach only the new base URL", newHits, oldHits)
	}
	if _, err := svc.RequestHTTP(activeID, "bob", "GET", "/api/x", nil, "", nil); err == nil || !strings.Contains(err.Error(), `no user "bob"`) {
		t.Errorf("removed user: err = %v, want a no-such-user error", err)
	}

	users, err := svc.HTTPChannelUsers(activeID)
	if err != nil || len(users) != 1 || users[0].Tags[0] != "renamed" {
		t.Errorf("users = %+v, err = %v, want alice tagged renamed", users, err)
	}

	if _, err := svc.ActivateHTTP("root-secret", inactive.ID, 3600e9); err != nil {
		t.Fatalf("ActivateHTTP: %v", err)
	}
	info, err := svc.HTTPChannelInfo(inactive.ID)
	if err != nil || info.BaseURL != newSrv.URL {
		t.Errorf("inactive channel after reactivation: info = %+v, err = %v, want base URL %s", info, err, newSrv.URL)
	}
}

func TestUpdateResterWrongPassphraseChangesNothing(t *testing.T) {
	srv := httptest.NewServer(http.NewServeMux())
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	edited := newEditableDomain("http://elsewhere.invalid", []vault.ResterUser{{Name: "mallory"}})
	if err := svc.UpdateRester("wrong-secret", "test-rester", edited.Rester()); err == nil {
		t.Fatal("expected an error for a wrong passphrase")
	}

	d, err := svc.GetRester("root-secret", "test-rester")
	if err != nil || d.Environments[0].BaseURL != srv.URL {
		t.Errorf("stored rester = %+v, err = %v, want it unchanged", d, err)
	}
	info, err := svc.HTTPChannelInfo(id)
	if err != nil || info.BaseURL != srv.URL {
		t.Errorf("channel info = %+v, err = %v, want it unchanged", info, err)
	}
}

func TestUpdateResterUnknownName(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetPassphrase("root-secret"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateRester("root-secret", "missing", vaulttest.Domain{BaseURL: "http://x"}.Rester()); err == nil {
		t.Error("expected an error updating a rester that was never stored")
	}
	if names, _ := svc.ListResters(); len(names) != 0 {
		t.Errorf("resters = %v, want none created by a failed update", names)
	}
}

func TestUpdateResterLeavesOtherRestersChannelsAlone(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})

	if err := svc.PutRester("root-secret", "other", newEditableDomain("http://other.invalid", []vault.ResterUser{{Name: "z"}}).Rester()); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateRester("root-secret", "other", newEditableDomain("http://changed.invalid", []vault.ResterUser{{Name: "z"}}).Rester()); err != nil {
		t.Fatal(err)
	}
	info, err := svc.HTTPChannelInfo(id)
	if err != nil || info.BaseURL != srv.URL {
		t.Errorf("channel on another rester: info = %+v, err = %v, want it untouched", info, err)
	}
}

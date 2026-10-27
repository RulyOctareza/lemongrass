// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

// setUpTwoTypeRester stores a rester with an api and a web side, one environment reaching both, and returns an api and a web channel on it.
func setUpTwoTypeRester(t *testing.T, apiURL string) (svc *testSvc, apiID, webID vault.ChannelID) {
	t.Helper()
	svc = newTestService(t)
	const root = "root-secret"
	if err := svc.SetPassphrase(root); err != nil {
		t.Fatal(err)
	}
	r := vault.Rester{
		API: &vault.ResterAPI{
			LoginEndpoint:   "/login",
			TokenPath:       "access_token",
			TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
			FixedTTLSeconds: 3600,
		},
		Web: &vault.ResterWeb{Login: []vault.LoginStep{
			{Action: vault.StepGoto, URL: "/login"},
			{Action: vault.StepFill, Selector: "#u", Field: "username"},
			{Action: vault.StepFill, Selector: "#p", Field: "password"},
			{Action: vault.StepClick, Selector: "button"},
		}},
		Environments: []vault.Environment{{Name: "staging", BaseURL: apiURL, Origin: "https://staging.example.test"}},
		Users: []vault.ResterUser{
			{Name: "alice", Fields: map[string]string{"username": "alice", "password": "s3cret-pw"}, Tags: []string{"admin"}, Environments: []string{"staging"}},
			{Name: "bob", Fields: map[string]string{"username": "bob", "password": "bob-pw"}, Environments: []string{"staging"}},
			{Name: "bot", Token: "s3cret-token", Environments: []string{"staging"}},
		},
	}
	if err := svc.PutRester(root, "shop", r); err != nil {
		t.Fatal(err)
	}
	api, err := svc.CreateHTTPChannel(root, "api", "shop", "staging", vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	web, err := svc.CreateHTTPChannel(root, "web", "shop", "staging", vault.TypeWeb, vault.HTTPScope{}, &vault.WebScope{Origins: []string{"https://sso.example.test"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return svc, api.ID, web.ID
}

func TestAPIOperationsRefuseAWebChannel(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, apiID, webID := setUpTwoTypeRester(t, srv.URL)

	if _, err := svc.RequestHTTP(apiID, "alice", "GET", "/api/x", nil, "", nil); err != nil {
		t.Fatalf("api channel request: %v", err)
	}
	hitsBefore := len(hits)

	_, err := svc.RequestHTTP(webID, "alice", "GET", "/api/x", nil, "", nil)
	if err == nil || !strings.HasPrefix(err.Error(), "restergate: ") || !strings.Contains(err.Error(), "web channel") || !strings.Contains(err.Error(), "screenshot") {
		t.Errorf("RequestHTTP on a web channel = %v, want a refusal naming the type and the web commands", err)
	}
	if _, err := svc.FlushHTTPTokens(webID, ""); err == nil || !strings.Contains(err.Error(), "web channel") {
		t.Errorf("FlushHTTPTokens on a web channel = %v, want a refusal", err)
	}
	if _, err := svc.DownloadHTTP(webID, "alice", "GET", "/api/x", nil, "", nil); err == nil || !strings.Contains(err.Error(), "web channel") {
		t.Errorf("DownloadHTTP on a web channel = %v, want a refusal", err)
	}
	if len(hits) != hitsBefore {
		t.Errorf("a refused call reached the upstream: %v", hits)
	}
}

func TestInfoAndUsersByEnvironmentAndType(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, apiID, webID := setUpTwoTypeRester(t, srv.URL)

	api, err := svc.HTTPChannelInfo(apiID)
	if err != nil {
		t.Fatal(err)
	}
	if api.Rester != "shop" || api.Environment != "staging" || api.Type != vault.TypeAPI || api.BaseURL != srv.URL || api.Origin != "" {
		t.Errorf("api info = %+v", api)
	}
	if len(api.Users) != 3 {
		t.Errorf("api users = %+v, want alice, bob and the token user", api.Users)
	}

	web, err := svc.HTTPChannelInfo(webID)
	if err != nil {
		t.Fatal(err)
	}
	if web.Type != vault.TypeWeb || web.Environment != "staging" || web.Origin != "https://staging.example.test" || web.BaseURL != "" {
		t.Errorf("web info = %+v", web)
	}
	if len(web.WebOrigins) != 1 || web.WebOrigins[0] != "https://sso.example.test" {
		t.Errorf("web origins = %v", web.WebOrigins)
	}
	if len(web.Users) != 2 || web.Users[0].Name != "alice" || web.Users[1].Name != "bob" {
		t.Errorf("web users = %+v, want only the login users", web.Users)
	}
	users, err := svc.HTTPChannelUsers(webID)
	if err != nil || len(users) != 2 {
		t.Errorf("HTTPChannelUsers on a web channel = %+v, %v, want two users", users, err)
	}
	raw, _ := json.Marshal(web)
	for _, secret := range []string{"s3cret-pw", "s3cret-token", "bob-pw"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("web info leaks %q: %s", secret, raw)
		}
	}
}

func TestWebLoginFields(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, apiID, webID := setUpTwoTypeRester(t, srv.URL)

	got, err := svc.WebLoginFields(webID, "Alice")
	if err != nil {
		t.Fatalf("WebLoginFields: %v", err)
	}
	if got.User != "alice" || got.Environment != "staging" || got.Origin != "https://staging.example.test" || got.Fields["password"] != "s3cret-pw" || len(got.Login) != 4 {
		t.Errorf("fields = %+v", got)
	}
	if got.BaseURL != srv.URL || got.TokenPlacement.Name != "Authorization" || got.TokenPath != "access_token" {
		t.Errorf("api hints = %+v", got)
	}

	if _, err := svc.WebLoginFields(webID, ""); err == nil || !strings.Contains(err.Error(), "alice") || !strings.Contains(err.Error(), "bob") || strings.Contains(err.Error(), "bot") {
		t.Errorf("empty user with several web users = %v, want the handles listed", err)
	}
	if _, err := svc.WebLoginFields(webID, "bot"); err == nil {
		t.Error("a token user got web login fields")
	}
	if _, err := svc.WebLoginFields(apiID, "alice"); err == nil || !strings.Contains(err.Error(), "not a web channel") {
		t.Errorf("WebLoginFields on an api channel = %v, want a refusal", err)
	}
	svc.forgetKey(webID)
	if _, err := svc.WebLoginFields(webID, "alice"); err == nil {
		t.Error("WebLoginFields on an inactive channel succeeded")
	}
}

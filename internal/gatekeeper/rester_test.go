// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestIPCResterAndWebChannel(t *testing.T) {
	client := startTestServer(t)

	r := vault.Rester{
		API: &vault.ResterAPI{LoginEndpoint: "/login", TokenPath: "token", TokenPlacement: vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization"}},
		Web: &vault.ResterWeb{Login: []vault.LoginStep{{Action: vault.StepFill, Selector: "#u", Field: "username"}}},
		Environments: []vault.Environment{
			{Name: "local", BaseURL: "http://localhost:8080", Origin: "http://localhost:3000"},
			{Name: "prod", BaseURL: "https://api.example.test"},
		},
		Users: []vault.ResterUser{{Name: "alice", Fields: map[string]string{"username": "alice"}, Environments: []string{"local", "prod"}}},
	}
	if err := client.PutRester(testRootSecret, "shop", r); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	if err := client.PutRester(testRootSecret, "shop", r); err == nil || !strings.Contains(err.Error(), `"shop"`) {
		t.Errorf("a second PutRester = %v, want it refused naming shop", err)
	}
	names, err := client.ListResters()
	if err != nil || len(names) != 1 || names[0] != "shop" {
		t.Errorf("ListResters = %v, %v", names, err)
	}

	web, err := client.CreateHTTPChannel(testRootSecret, "w", "shop", "local", vault.TypeWeb, vault.HTTPScope{}, &vault.WebScope{Origins: []string{"https://sso.example.test"}}, time.Hour)
	if err != nil {
		t.Fatalf("CreateHTTPChannel web: %v", err)
	}
	if web.Type != vault.TypeWeb || web.Environment != "local" || web.Rester != "shop" || web.WebScope == nil {
		t.Errorf("web channel = %+v", web)
	}
	if _, err := client.CreateHTTPChannel(testRootSecret, "w", "shop", "prod", vault.TypeWeb, vault.HTTPScope{}, nil, time.Hour); err == nil {
		t.Error("a web channel on an environment with no origin was created")
	}

	info, err := client.HTTPChannelInfo(web.ID)
	if err != nil || info.Type != vault.TypeWeb || info.Environment != "local" || info.Origin != "http://localhost:3000" || info.Status != "active" {
		t.Errorf("info = %+v, %v", info, err)
	}
	fields, err := client.WebLoginFields(web.ID, "")
	if err != nil || fields.User != "alice" || fields.Fields["username"] != "alice" || fields.Origin != "http://localhost:3000" {
		t.Errorf("WebLoginFields = %+v, %v", fields, err)
	}
	if _, err := client.RequestHTTP(web.ID, "alice", "GET", "/x", nil, "", nil, ""); err == nil || !strings.Contains(err.Error(), "web channel") {
		t.Errorf("RequestHTTP on a web channel = %v, want a refusal", err)
	}

	api, err := client.CreateHTTPChannel(testRootSecret, "a", "shop", "prod", vault.TypeAPI, vault.HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatalf("CreateHTTPChannel api: %v", err)
	}
	if _, err := client.WebLoginFields(api.ID, "alice"); err == nil {
		t.Error("WebLoginFields on an api channel succeeded")
	}
	chans, err := client.ListHTTPChannels()
	if err != nil || len(chans) != 2 {
		t.Errorf("ListHTTPChannels = %v, %v", chans, err)
	}

	if err := client.UpdateRester(testRootSecret, "shop", vault.Rester{API: r.API, Environments: r.Environments[:1], Users: r.Users}); err == nil {
		t.Error("an edit removing the web side and an environment in use was accepted")
	}
	got, err := client.GetRester(testRootSecret, "shop")
	if err != nil || got.Web == nil || len(got.Environments) != 2 {
		t.Errorf("rester after a refused edit = %+v, %v", got, err)
	}
	if err := client.TestResterLogin(vault.ResterAPI{}, vault.Environment{Name: "x"}, vault.ResterUser{Token: "t"}); err != nil {
		t.Errorf("TestResterLogin for a fixed-ttl token user: %v", err)
	}
}

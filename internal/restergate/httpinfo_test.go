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

func TestHTTPChannelInfoActive(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	users := []vault.ResterUser{
		{Name: "alice", Fields: map[string]string{"password": "s3cret-pw"}},
		{Name: "bob", Token: "s3cret-token"},
	}
	svc, id := setUpHTTPChannel(t, srv, users, []string{"GET", "POST"})

	info, err := svc.HTTPChannelInfo(id)
	if err != nil {
		t.Fatalf("HTTPChannelInfo: %v", err)
	}
	if info.Status != HTTPStatusActive || info.Name != "test-channel" || info.Rester != "test-rester" {
		t.Errorf("info = %+v, want an active test-channel on test-rester", info)
	}
	if info.BaseURL != srv.URL {
		t.Errorf("BaseURL = %q, want %q", info.BaseURL, srv.URL)
	}
	if info.RemainingSeconds < 3590 || info.RemainingSeconds > 3600 {
		t.Errorf("RemainingSeconds = %d, want about 3600", info.RemainingSeconds)
	}
	if len(info.Methods) != 2 || len(info.Users) != 2 || info.Users[0].Name != "alice" || info.Users[1].Name != "bob" {
		t.Errorf("info = %+v, want two methods and users alice and bob", info)
	}

	raw, _ := json.Marshal(info)
	for _, secret := range []string{"s3cret-pw", "s3cret-token"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("info JSON leaks %q: %s", secret, raw)
		}
	}
}

func TestHTTPChannelInfoInactive(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})
	svc.forgetKey(id)

	info, err := svc.HTTPChannelInfo(id)
	if err != nil {
		t.Fatalf("HTTPChannelInfo: %v", err)
	}
	if info.Status != HTTPStatusInactive || info.BaseURL != "" || len(info.Users) != 0 {
		t.Errorf("info = %+v, want inactive with no base URL or users", info)
	}
	if info.Rester != "test-rester" || len(info.Methods) != 1 {
		t.Errorf("info = %+v, want the metadata that needs no key", info)
	}
}

func TestHTTPChannelInfoExpired(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(newLoginMux(&hits))
	defer srv.Close()
	svc, id := setUpHTTPChannel(t, srv, []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}}, []string{"GET"})
	if _, err := svc.ActivateHTTP("root-secret", id, -time.Minute); err != nil {
		t.Fatal(err)
	}

	info, err := svc.HTTPChannelInfo(id)
	if err != nil {
		t.Fatalf("HTTPChannelInfo on an expired channel returned an error: %v", err)
	}
	if info.Status != HTTPStatusExpired || info.RemainingSeconds != 0 || info.BaseURL != "" || info.Name != "test-channel" {
		t.Errorf("info = %+v, want expired, zero remaining, no base URL, name kept", info)
	}
}

func TestHTTPChannelInfoUnknownChannel(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.HTTPChannelInfo("nope"); err == nil {
		t.Error("expected an error for an unknown channel")
	}
}

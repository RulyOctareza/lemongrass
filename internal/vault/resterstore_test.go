// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"strings"
	"testing"
)

func TestPutListDeleteRester(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	d := NewAPIRester("https://api.example.test", ResterAPI{LoginEndpoint: "/login", TokenPath: "access_token"}, nil)
	if err := svc.PutRester("root-secret", "staging", d); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	if err := svc.PutCredential("root-secret", "some-db", []byte("postgres://x")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	resters, err := svc.ListResters()
	if err != nil {
		t.Fatalf("ListResters: %v", err)
	}
	if len(resters) != 1 || resters[0] != "staging" {
		t.Errorf("ListResters() = %v, want [\"staging\"]", resters)
	}

	conns, err := svc.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) != 1 || conns[0] != "some-db" {
		t.Errorf("ListConnections() = %v, want [\"some-db\"] (resters excluded)", conns)
	}

	got, err := svc.GetRester("root-secret", "staging")
	if err != nil {
		t.Fatalf("GetRester: %v", err)
	}
	if got.Environments[0].BaseURL != d.Environments[0].BaseURL || got.API.LoginEndpoint != d.API.LoginEndpoint {
		t.Errorf("GetRester() = %+v, want %+v", got, d)
	}

	if err := svc.DeleteRester("staging"); err != nil {
		t.Fatalf("DeleteRester: %v", err)
	}
	resters, err = svc.ListResters()
	if err != nil {
		t.Fatalf("ListResters after delete: %v", err)
	}
	if len(resters) != 0 {
		t.Errorf("ListResters() after delete = %v, want empty", resters)
	}
}

func TestPutResterRefusesAStoredName(t *testing.T) {
	svc := openTestService(t)
	first := NewAPIRester("https://one.example.test", ResterAPI{}, []ResterUser{{Name: "alice", Token: "t"}})
	if err := svc.PutRester(testRootSecret, "staging", first); err != nil {
		t.Fatalf("first PutRester: %v", err)
	}

	second := NewAPIRester("https://two.example.test", ResterAPI{}, nil)
	err := svc.PutRester(testRootSecret, "staging", second)
	if err == nil || !strings.HasPrefix(err.Error(), "vault: ") || !strings.Contains(err.Error(), `"staging"`) {
		t.Fatalf("PutRester on a stored name = %v, want a vault error naming it", err)
	}
	got, err := svc.GetRester(testRootSecret, "staging")
	if err != nil || got.Environments[0].BaseURL != "https://one.example.test" || len(got.Users) != 1 {
		t.Errorf("stored rester = %+v, %v, want the first one unchanged", got, err)
	}

	if err := svc.PutRester(testRootSecret, "other", second); err != nil {
		t.Errorf("PutRester on a new name: %v", err)
	}
	if err := svc.UpdateRester(testRootSecret, "staging", second); err != nil {
		t.Fatalf("UpdateRester on a stored name: %v", err)
	}
	got, _ = svc.GetRester(testRootSecret, "staging")
	if got.Environments[0].BaseURL != "https://two.example.test" {
		t.Errorf("UpdateRester left %+v, want the replacement", got)
	}
	if err := svc.UpdateRester(testRootSecret, "missing", second); err == nil {
		t.Error("UpdateRester on an unknown name = nil, want an error")
	}
}

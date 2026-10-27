// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"strings"
	"testing"
)

func TestUserHandle(t *testing.T) {
	cases := map[string]string{
		"John Doe":       "john_doe",
		"  john   DOE ":  "john_doe",
		"john_doe":       "john_doe",
		"Alice":          "alice",
		"tab\tseparated": "tab_separated",
		"":               "",
	}
	for in, want := range cases {
		if got := UserHandle(in); got != want {
			t.Errorf("UserHandle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPutResterRejectsUsersWithTheSameHandle(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	d := NewAPIRester("https://api.example.test", ResterAPI{}, []ResterUser{{Name: "John Doe"}, {Name: "john_doe"}})

	err = svc.PutRester("root-secret", "staging", d)
	if err == nil || !strings.Contains(err.Error(), "John Doe") || !strings.Contains(err.Error(), "john_doe") {
		t.Fatalf("PutRester err = %v, want a collision naming both users", err)
	}
	if names, _ := svc.ListResters(); len(names) != 0 {
		t.Errorf("ListResters() = %v, want nothing stored after a refused save", names)
	}

	d.Users[1].Name = "John DOE"
	if err := svc.PutRester("root-secret", "staging", d); err == nil {
		t.Error("PutRester accepted two users that differ only in case")
	}

	d.Users[1].Name = "jane doe"
	if err := svc.PutRester("root-secret", "staging", d); err != nil {
		t.Errorf("PutRester with distinct handles: %v", err)
	}
}

func TestUpdateResterRejectsUsersWithTheSameHandle(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	d := NewAPIRester("https://api.example.test", ResterAPI{}, []ResterUser{{Name: "John Doe"}})
	if err := svc.PutRester("root-secret", "staging", d); err != nil {
		t.Fatalf("PutRester: %v", err)
	}

	d.Users = append(d.Users, ResterUser{Name: "john_doe", Environments: []string{DefaultEnvironment}})
	if err := svc.UpdateRester("root-secret", "staging", d); err == nil {
		t.Fatal("UpdateRester accepted a colliding user")
	}
	got, err := svc.GetRester("root-secret", "staging")
	if err != nil || len(got.Users) != 1 {
		t.Errorf("stored rester = %+v, %v, want the original single user kept", got, err)
	}
}

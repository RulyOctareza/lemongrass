// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"testing"
	"time"
)

func TestListChannelsNewestFirst(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	var created []ChannelID
	for i := 0; i < 12; i++ {
		c, err := svc.CreateChannel(testRootSecret, "ch", "app-backend", fullScope(), 5*time.Minute)
		if err != nil {
			t.Fatalf("CreateChannel %d: %v", i, err)
		}
		created = append(created, c.ID)
		time.Sleep(2 * time.Millisecond)
	}

	got, err := svc.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(got) != len(created) {
		t.Fatalf("len(ListChannels()) = %d, want %d", len(got), len(created))
	}
	for i, c := range got {
		want := created[len(created)-1-i]
		if c.ID != want {
			t.Fatalf("ListChannels()[%d] = %s, want %s (newest first)", i, c.ID, want)
		}
	}
}

func TestListHTTPChannelsNewestFirst(t *testing.T) {
	svc := openTestService(t)
	d := NewAPIRester("https://api.example.test", ResterAPI{LoginEndpoint: "/login", TokenPath: "access_token"}, []ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}})
	if err := svc.PutRester(testRootSecret, "staging", d); err != nil {
		t.Fatalf("PutRester: %v", err)
	}

	var created []ChannelID
	for i := 0; i < 12; i++ {
		c, err := svc.CreateHTTPChannel(testRootSecret, "ch", "staging", DefaultEnvironment, TypeAPI, HTTPScope{Methods: []string{"GET"}}, nil, 5*time.Minute)
		if err != nil {
			t.Fatalf("CreateHTTPChannel %d: %v", i, err)
		}
		created = append(created, c.ID)
		time.Sleep(2 * time.Millisecond)
	}

	got, err := svc.ListHTTPChannels()
	if err != nil {
		t.Fatalf("ListHTTPChannels: %v", err)
	}
	if len(got) != len(created) {
		t.Fatalf("len(ListHTTPChannels()) = %d, want %d", len(got), len(created))
	}
	for i, c := range got {
		want := created[len(created)-1-i]
		if c.ID != want {
			t.Fatalf("ListHTTPChannels()[%d] = %s, want %s (newest first)", i, c.ID, want)
		}
	}
}

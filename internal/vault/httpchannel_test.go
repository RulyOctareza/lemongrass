// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import "testing"

func TestHTTPScopeAllowRequiresMethodGrant(t *testing.T) {
	s := HTTPScope{Methods: []string{"GET", "POST"}}
	if !s.Allow("GET", "/api/orders/42") {
		t.Error("GET should be allowed: granted, no exclusion")
	}
	if !s.Allow("post", "/api/orders") {
		t.Error("method match should be case-insensitive")
	}
	if s.Allow("DELETE", "/api/orders/42") {
		t.Error("DELETE should be denied: not in Methods")
	}
}

func TestHTTPScopeExclusionMatchesMethodAndPathTogether(t *testing.T) {
	s := HTTPScope{
		Methods: []string{"PUT"},
		Exclusions: []MethodPath{
			{Method: "PUT", PathPattern: "/users/*/change-password"},
		},
	}
	if s.Allow("PUT", "/users/42/change-password") {
		t.Error("PUT to the excluded path should be denied")
	}
	if !s.Allow("PUT", "/users/42/profile") {
		t.Error("PUT to a different path under the same granted method should still be allowed")
	}
}

func TestHTTPScopeExclusionIsMethodSpecific(t *testing.T) {
	s := HTTPScope{
		Methods: []string{"GET", "PUT"},
		Exclusions: []MethodPath{
			{Method: "PUT", PathPattern: "/secrets/*"},
		},
	}
	if !s.Allow("GET", "/secrets/42") {
		t.Error("GET on an excluded-for-PUT path should still be allowed: exclusion is method+path together, not path alone")
	}
	if s.Allow("PUT", "/secrets/42") {
		t.Error("PUT on the excluded path should be denied")
	}
}

func TestHTTPScopePatternDoesNotCrossSegments(t *testing.T) {
	s := HTTPScope{
		Methods: []string{"DELETE"},
		Exclusions: []MethodPath{
			{Method: "DELETE", PathPattern: "/orders/*"},
		},
	}
	if s.Allow("DELETE", "/orders/42") {
		t.Error("PathPattern's * should match a single path segment, excluding /orders/42")
	}
	if !s.Allow("DELETE", "/orders/42/items/7") {
		t.Error("PathPattern's * should not cross a /, so a deeper path stays allowed")
	}
}

func TestHTTPChannelExpired(t *testing.T) {
	c, err := NewChannelID()
	if err != nil {
		t.Fatalf("NewChannelID: %v", err)
	}
	hc := HTTPChannel{ID: c}
	if hc.Kind() != "http" {
		t.Errorf("HTTPChannel.Kind() = %q, want \"http\"", hc.Kind())
	}
}

func TestChannelKind(t *testing.T) {
	c := Channel{}
	if c.Kind() != "db" {
		t.Errorf("Channel{}.Kind() = %q, want \"db\"", c.Kind())
	}
}

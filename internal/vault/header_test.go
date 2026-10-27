// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"strings"
	"testing"
)

func headerRester(headers ...Header) Rester {
	return NewAPIRester("https://api.example.test", ResterAPI{
		TokenPlacement: TokenPlacement{Kind: PlacementHeader, Name: "x-api-key"},
		Headers:        headers,
	}, []ResterUser{{Name: "alice", Token: "secret"}})
}

func TestPutResterStoresHeaders(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	d := headerRester(Header{Name: "anthropic-version", Value: "2023-06-01"}, Header{Name: "X-Tenant", Value: ""})
	if err := svc.PutRester("root-secret", "staging", d); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	got, err := svc.GetRester("root-secret", "staging")
	if err != nil {
		t.Fatalf("GetRester: %v", err)
	}
	if len(got.API.Headers) != 2 || got.API.Headers[0] != d.API.Headers[0] || got.API.Headers[1] != d.API.Headers[1] {
		t.Errorf("stored Headers = %v, want %v", got.API.Headers, d.API.Headers)
	}
}

func TestPutResterWithoutHeaders(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.PutRester("root-secret", "staging", headerRester()); err != nil {
		t.Fatalf("PutRester: %v", err)
	}
	got, err := svc.GetRester("root-secret", "staging")
	if err != nil {
		t.Fatalf("GetRester: %v", err)
	}
	if len(got.API.Headers) != 0 {
		t.Errorf("stored Headers = %v, want none", got.API.Headers)
	}
}

func TestPutResterRejectsBadHeaders(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	cases := []struct {
		name    string
		headers []Header
		want    string
	}{
		{"empty name", []Header{{Name: "", Value: "v"}}, "header name is empty"},
		{"space in name", []Header{{Name: "X Tenant", Value: "v"}}, `header name "X Tenant" has a character not allowed`},
		{"newline in value", []Header{{Name: "X-Tenant", Value: "a\r\nX-Evil: 1"}}, `header "X-Tenant" has a value that is not a single line`},
		{"proxy header", []Header{{Name: "content-type", Value: "text/plain"}}, `header "content-type" is set by the proxy`},
		{"token header", []Header{{Name: "X-API-Key", Value: "other"}}, `header "X-API-Key" is set by the proxy`},
		{"repeated", []Header{{Name: "X-Tenant", Value: "1"}, {Name: "x-tenant", Value: "2"}}, `header "x-tenant" is set more than once`},
	}
	for _, c := range cases {
		err := svc.PutRester("root-secret", "staging", headerRester(c.headers...))
		if err == nil || !strings.HasPrefix(err.Error(), "vault: ") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: PutRester = %v, want an error containing %q", c.name, err, c.want)
		}
	}
}

func TestReservesHeaderFollowsTokenPlacement(t *testing.T) {
	d := ResolvedRester{TokenPlacement: TokenPlacement{Kind: PlacementHeader, Name: "x-api-key"}}
	if !d.ReservesHeader("x-api-key") || !d.ReservesHeader("HOST") {
		t.Errorf("ReservesHeader missed the token header or a proxy header")
	}
	d.TokenPlacement = TokenPlacement{Kind: PlacementQuery, Name: "x-api-key"}
	if d.ReservesHeader("x-api-key") {
		t.Errorf("ReservesHeader(x-api-key) with query placement = true, want false")
	}
}

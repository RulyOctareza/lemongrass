package vault

import (
	"strings"
	"testing"
)

func headerDomain(headers ...Header) Domain {
	return Domain{
		BaseURL:        "https://api.example.test",
		TokenPlacement: TokenPlacement{Kind: PlacementHeader, Name: "x-api-key"},
		Users:          []DomainUser{{Name: "alice", Token: "secret"}},
		Headers:        headers,
	}
}

func TestPutDomainStoresHeaders(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	d := headerDomain(Header{Name: "anthropic-version", Value: "2023-06-01"}, Header{Name: "X-Tenant", Value: ""})
	if err := svc.PutDomain("root-secret", "staging", d); err != nil {
		t.Fatalf("PutDomain: %v", err)
	}
	got, err := svc.getDomain("root-secret", "staging")
	if err != nil {
		t.Fatalf("getDomain: %v", err)
	}
	if len(got.Headers) != 2 || got.Headers[0] != d.Headers[0] || got.Headers[1] != d.Headers[1] {
		t.Errorf("stored Headers = %v, want %v", got.Headers, d.Headers)
	}
}

func TestPutDomainWithoutHeaders(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.PutDomain("root-secret", "staging", headerDomain()); err != nil {
		t.Fatalf("PutDomain: %v", err)
	}
	got, err := svc.getDomain("root-secret", "staging")
	if err != nil {
		t.Fatalf("getDomain: %v", err)
	}
	if len(got.Headers) != 0 {
		t.Errorf("stored Headers = %v, want none", got.Headers)
	}
}

func TestPutDomainRejectsBadHeaders(t *testing.T) {
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
		err := svc.PutDomain("root-secret", "staging", headerDomain(c.headers...))
		if err == nil || !strings.HasPrefix(err.Error(), "vault: ") || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: PutDomain = %v, want an error containing %q", c.name, err, c.want)
		}
	}
}

func TestReservesHeaderFollowsTokenPlacement(t *testing.T) {
	d := headerDomain()
	if !d.ReservesHeader("x-api-key") || !d.ReservesHeader("HOST") {
		t.Errorf("ReservesHeader missed the token header or a proxy header")
	}
	d.TokenPlacement = TokenPlacement{Kind: PlacementQuery, Name: "x-api-key"}
	if d.ReservesHeader("x-api-key") {
		t.Errorf("ReservesHeader(x-api-key) with query placement = true, want false")
	}
}

package vault

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TokenPlacementKind says which part of an outgoing request carries the token.
type TokenPlacementKind string

const (
	PlacementHeader TokenPlacementKind = "header"
	PlacementCookie TokenPlacementKind = "cookie"
	PlacementQuery  TokenPlacementKind = "query"
)

// TokenPlacement says where an obtained token is injected on an outgoing call. Prefix is only
// meaningful for PlacementHeader, e.g. Name "Authorization", Prefix "Bearer ".
type TokenPlacement struct {
	Kind   TokenPlacementKind
	Name   string
	Prefix string
}

// Domain is one HTTP backend's shape: how to log a user in, where the token lives in the
// response, how to learn its lifetime, and where to inject it -- one Store entry holding many
// users, unlike a db Connection's one name to one connection string.
type Domain struct {
	BaseURL       string
	LoginEndpoint string // empty when every user on this domain is bring-your-own-token
	TokenPath     string // JSON path into the login response, e.g. "access_token" or "data.token"

	// TTLOrigin is a JSON path into the login response (e.g. "expires_in"), the literal
	// "jwt-exp" to decode the token itself as a JWT and read its exp claim, or "" to fall
	// back to FixedTTLSeconds.
	TTLOrigin       string
	FixedTTLSeconds int64

	TokenPlacement TokenPlacement
	Users          []DomainUser
}

// DomainUser is a login user (Fields carries whatever LoginEndpoint needs, e.g.
// username/password) or a bring-your-own-token user (Token is pre-supplied and LoginEndpoint
// is never called for it) -- never both. Tags are free text a human sets to say what the user
// is for, and are the only part of a user a model is ever shown besides its name. FieldTypes
// maps a field to FieldNumber or FieldBoolean, and a field with no entry is text.
type DomainUser struct {
	Name       string
	Fields     map[string]string
	FieldTypes map[string]string
	Token      string
	Tags       []string
}

func (u DomainUser) IsBYOT() bool {
	return u.Token != ""
}

// UserHandle is the name a model types for a user: trimmed, lowercased, whitespace runs joined by one underscore.
func UserHandle(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "_")
}

func checkUserHandles(users []DomainUser) error {
	seen := make(map[string]string, len(users))
	for _, u := range users {
		h := UserHandle(u.Name)
		if prev, ok := seen[h]; ok {
			return fmt.Errorf("vault: users %q and %q both map to the handle %q", prev, u.Name, h)
		}
		seen[h] = u.Name
	}
	return nil
}

const domainKeyPrefix = "domains-"

func domainKey(name string) string {
	return domainKeyPrefix + name
}

// PutDomain encrypts d under the root key and stores it as name, alongside credentials in the
// same Store but under a distinct key prefix so domain and db connection names never collide.
func (s *Service) PutDomain(rootSecret, name string, d Domain) error {
	if err := checkUserHandles(d.Users); err != nil {
		return err
	}
	if err := checkLoginFields(d.Users); err != nil {
		return err
	}
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)

	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("vault: encoding domain %s: %w", name, err)
	}
	return s.creds.Put(domainKey(name), rootKey, b)
}

// ListDomains lists stored domain names only, never a decrypted value, without needing a root secret.
func (s *Service) ListDomains() ([]string, error) {
	names, err := s.creds.List()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if rest, ok := strings.CutPrefix(n, domainKeyPrefix); ok {
			out = append(out, rest)
		}
	}
	return out, nil
}

// DeleteDomain removes a stored domain. It succeeds even if name was never stored.
func (s *Service) DeleteDomain(name string) error {
	return s.creds.Delete(domainKey(name))
}

// getDomain decrypts name's stored domain config.
func (s *Service) getDomain(rootSecret, name string) (Domain, error) {
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return Domain{}, err
	}
	defer Zero(rootKey)

	b, err := s.creds.Get(domainKey(name), rootKey)
	if err != nil {
		return Domain{}, fmt.Errorf("vault: reading domain %s: %w", name, err)
	}
	defer Zero(b)

	var d Domain
	if err := json.Unmarshal(b, &d); err != nil {
		return Domain{}, fmt.Errorf("vault: decoding domain %s: %w", name, err)
	}
	return d, nil
}

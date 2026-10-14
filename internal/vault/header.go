package vault

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Header is one extra request header, set on a domain by a human or on a single call by a model.
type Header struct {
	Name  string
	Value string
}

// proxyHeaders are set by the proxy for every call and cannot be set by a domain or a model.
var proxyHeaders = []string{"Host", "Content-Type", "Content-Length", "Transfer-Encoding", "Connection"}

// CheckHeader reports whether h has a token name and a value with no CR, LF or NUL.
func CheckHeader(h Header) error {
	if h.Name == "" {
		return errors.New("header name is empty")
	}
	for _, r := range h.Name {
		if !isTokenRune(r) {
			return fmt.Errorf("header name %q has a character not allowed in a header name", h.Name)
		}
	}
	if strings.ContainsAny(h.Value, "\r\n\x00") {
		return fmt.Errorf("header %q has a value that is not a single line", h.Name)
	}
	return nil
}

func isTokenRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

// ReservesHeader reports whether name is set by the proxy on a call to d: a proxy header or d's token header.
func (d Domain) ReservesHeader(name string) bool {
	for _, p := range proxyHeaders {
		if strings.EqualFold(p, name) {
			return true
		}
	}
	return d.TokenPlacement.Kind == PlacementHeader && strings.EqualFold(d.TokenPlacement.Name, name)
}

// SetsHeader reports whether one of d's own headers is named name.
func (d Domain) SetsHeader(name string) bool {
	for _, h := range d.Headers {
		if strings.EqualFold(h.Name, name) {
			return true
		}
	}
	return false
}

func checkDomainHeaders(d Domain) error {
	seen := make(map[string]bool, len(d.Headers))
	for _, h := range d.Headers {
		if err := CheckHeader(h); err != nil {
			return fmt.Errorf("vault: %w", err)
		}
		if d.ReservesHeader(h.Name) {
			return fmt.Errorf("vault: header %q is set by the proxy and cannot be set on a domain", h.Name)
		}
		key := http.CanonicalHeaderKey(h.Name)
		if seen[key] {
			return fmt.Errorf("vault: header %q is set more than once", h.Name)
		}
		seen[key] = true
	}
	return nil
}

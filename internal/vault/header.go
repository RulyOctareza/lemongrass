// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Header is one extra request header, set on a rester by a human or on a single call by a model.
type Header struct {
	Name  string
	Value string
}

// proxyHeaders are set by the proxy for every call and cannot be set by a rester or a model.
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

func reservesHeader(p TokenPlacement, name string) bool {
	for _, h := range proxyHeaders {
		if strings.EqualFold(h, name) {
			return true
		}
	}
	return p.Kind == PlacementHeader && strings.EqualFold(p.Name, name)
}

// checkHeaderList refuses headers that are malformed, set by the proxy or repeated. what names the owner in the error.
func checkHeaderList(headers []Header, p TokenPlacement, what string) error {
	seen := make(map[string]bool, len(headers))
	for _, h := range headers {
		if err := CheckHeader(h); err != nil {
			return fmt.Errorf("vault: %s: %w", what, err)
		}
		if reservesHeader(p, h.Name) {
			return fmt.Errorf("vault: %s: header %q is set by the proxy and cannot be set on a rester", what, h.Name)
		}
		key := http.CanonicalHeaderKey(h.Name)
		if seen[key] {
			return fmt.Errorf("vault: %s: header %q is set more than once", what, h.Name)
		}
		seen[key] = true
	}
	return nil
}

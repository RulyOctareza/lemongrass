// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"encoding/json"
	"path"
	"strings"
	"time"
)

// channelLike is implemented by both Channel and HTTPChannel, letting shared machinery (audit
// stamping, an expiry sweep) work across both kinds without a type switch.
type channelLike interface {
	Kind() string
	Expired(now time.Time) bool
}

var (
	_ channelLike = Channel{}
	_ channelLike = HTTPChannel{}
)

// MethodPath pairs an HTTP method with a path pattern, matched together as one exclusion so a
// verb can be blocked on one endpoint without losing access to other verbs on that same path.
type MethodPath struct {
	Method      string
	PathPattern string
}

// HTTPScope is a method allow-list plus an opt-in exclusion list that overrides even a granted
// method, the REST analog of Scope's table allow-list.
type HTTPScope struct {
	Methods    []string
	Exclusions []MethodPath
}

// Allow reports whether method is granted and no exclusion matches method and requestPath
// together. PathPattern uses path.Match semantics, so "*" matches one path segment, not "/".
func (s HTTPScope) Allow(method, requestPath string) bool {
	if !containsMethod(s.Methods, method) {
		return false
	}
	for _, ex := range s.Exclusions {
		if !strings.EqualFold(ex.Method, method) {
			continue
		}
		if matched, err := path.Match(ex.PathPattern, requestPath); err == nil && matched {
			return false
		}
	}
	return true
}

func containsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

// WebScope is a web channel's limits: the origins the page may reach besides its environment's
// own, and exclusions that block requests the page sends to the environment's base URL. There
// is no method allow-list since a page sends GETs and preflights on its own.
type WebScope struct {
	Origins    []string
	Exclusions []MethodPath
}

// HTTPChannel is the HTTP-channel sibling of Channel: a genuinely separate type since a
// rester's many-users shape and method+exclusion scope don't fit Channel's
// DBName/Scope/Port shape. A channel is one environment of one rester and one type: Scope
// limits an api channel and WebScope a web one.
type HTTPChannel struct {
	ID          ChannelID
	Name        string
	Rester      string
	Environment string
	Type        string
	Scope       HTTPScope
	WebScope    *WebScope
	Salt        []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// UnmarshalJSON reads a channel stored before resters existed: its rester sat under the key
// Domain, and it has no environment or type, which read as default and api.
func (c *HTTPChannel) UnmarshalJSON(b []byte) error {
	type plain HTTPChannel
	aux := struct {
		plain
		Domain string
	}{}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*c = HTTPChannel(aux.plain)
	if c.Rester == "" {
		c.Rester = aux.Domain
	}
	if c.Environment == "" {
		c.Environment = DefaultEnvironment
	}
	if c.Type == "" {
		c.Type = TypeAPI
	}
	return nil
}

func (c HTTPChannel) Expired(now time.Time) bool {
	return !now.Before(c.ExpiresAt)
}

func (c HTTPChannel) Kind() string {
	return "http"
}

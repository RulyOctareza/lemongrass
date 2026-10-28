// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"encoding/json"
	"fmt"
	"slices"
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

// Channel types: what a channel grants. A channel is one type and one environment, never both.
const (
	TypeAPI = "api"
	TypeWeb = "web"
	TypeApp = "app"
)

// DefaultEnvironment is the environment a pre-environment domain or channel reads as.
const DefaultEnvironment = "default"

// Rester is one backend system a human stored: the environments it runs in, the users that log
// in to it, and how a call (API side), a browser (web side) or a phone app (app side) logs them in. One Store entry
// holds the whole thing, so an edit is a read-modify-write of the record.
type Rester struct {
	API          *ResterAPI
	Web          *ResterWeb
	App          *ResterApp
	Environments []Environment
	Users        []ResterUser
}

// ResterAPI is the login shape every environment shares: how to log a user in, where the token
// lives in the response, how to learn its lifetime, and where to inject it.
type ResterAPI struct {
	LoginEndpoint string // empty when every user is bring-your-own-token
	TokenPath     string // JSON path into the login response, e.g. "access_token" or "data.token"

	// TTLOrigin is a JSON path into the login response (e.g. "expires_in"), the literal
	// "jwt-exp" to decode the token itself as a JWT and read its exp claim, or "" to fall
	// back to FixedTTLSeconds.
	TTLOrigin       string
	FixedTTLSeconds int64

	TokenPlacement TokenPlacement

	// Headers go on the login call and every proxied call, and a model never sees them.
	Headers []Header
}

// ResterWeb is the browser login every environment shares, as ordered steps.
type ResterWeb struct {
	Login []LoginStep
}

// ResterApp is the phone app login every environment shares: the app's package name and ordered
// steps typed over adb.
type ResterApp struct {
	Package string
	Login   []AppStep
}

// App login step actions. A fill step names a key of the user's fields, never a literal value.
const (
	AppStepLaunch = "launch"
	AppStepTap    = "tap"
	AppStepFill   = "fill"
	AppStepKey    = "key"
	AppStepWait   = "wait"
)

// AppStep is one step of an app login. launch starts the package, tap and wait find an element
// by Text or ID, fill finds one by Text or ID and types the user's Field into it, key sends the
// key named Key. TimeoutSeconds bounds the step, 0 meaning the executor's default.
type AppStep struct {
	Action         string
	Text           string
	ID             string
	Field          string
	Key            string
	TimeoutSeconds int
}

// Login step actions. A fill step names a key of the user's fields, never a literal value.
const (
	StepGoto  = "goto"
	StepFill  = "fill"
	StepClick = "click"
	StepWait  = "wait"
)

// LoginStep is one step of a web login. goto uses URL, fill uses Selector and Field, click uses
// Selector, wait uses Selector or a URL pattern in URL. TimeoutSeconds bounds the step, 0 meaning the engine's default.
type LoginStep struct {
	Action         string
	URL            string
	Selector       string
	Field          string
	TimeoutSeconds int
}

// Environment is one place a rester runs (local, staging, prod). BaseURL is its API address and
// Origin its web address; an empty one means the environment has no such side. Headers are
// extra API headers that win over the shared ones on a name clash.
type Environment struct {
	Name    string
	BaseURL string
	Origin  string
	Headers []Header
}

// ResterUser is a login user (Fields carries whatever the login needs, e.g. username/password)
// or a bring-your-own-token user (Token is pre-supplied and no login ever runs for it) -- never
// both. Tags are free text a human sets to say what the user is for, and are the only part of a
// user a model is ever shown besides its name. FieldTypes maps a field to FieldNumber or
// FieldBoolean, and a field with no entry is text. Environments lists where the user exists and
// Overrides replaces credentials per environment.
type ResterUser struct {
	Name         string
	Fields       map[string]string
	FieldTypes   map[string]string
	Token        string
	Tags         []string
	Environments []string
	Overrides    map[string]UserOverride
}

// UserOverride replaces a user's credentials in one environment: Fields and FieldTypes key by
// key, Token as a whole.
type UserOverride struct {
	Fields     map[string]string
	FieldTypes map[string]string
	Token      string
}

// IsBYOT reports whether u carries a token. On a stored user that is the shared token; the
// effective user of an environment is the one to ask.
func (u ResterUser) IsBYOT() bool {
	return u.Token != ""
}

// UserHandle is the name a model types for a user: trimmed, lowercased, whitespace runs joined by one underscore.
func UserHandle(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "_")
}

// NewAPIRester is the plain API rester: one environment named default holding baseURL, with every user assigned to it.
func NewAPIRester(baseURL string, api ResterAPI, users []ResterUser) Rester {
	out := make([]ResterUser, len(users))
	for i, u := range users {
		u.Environments = []string{DefaultEnvironment}
		out[i] = u
	}
	return Rester{API: &api, Environments: []Environment{{Name: DefaultEnvironment, BaseURL: baseURL}}, Users: out}
}

func (r Rester) environment(name string) (Environment, bool) {
	for _, e := range r.Environments {
		if e.Name == name {
			return e, true
		}
	}
	return Environment{}, false
}

func (u ResterUser) assignedTo(env string) bool {
	return slices.Contains(u.Environments, env)
}

const resterKeyPrefix = "domains-"

func resterKey(name string) string {
	return resterKeyPrefix + name
}

// putRester encrypts r under the root key and stores it as name, overwriting. Resters keep the
// "domains-" key prefix of the domains they replace, so no stored file is renamed and a rester
// name can never collide with a db connection name.
func (s *Service) putRester(rootSecret, name string, r Rester) error {
	if err := r.validateNamed(name); err != nil {
		return err
	}
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return err
	}
	defer Zero(rootKey)

	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("vault: encoding rester %s: %w", name, err)
	}
	defer Zero(b)
	return s.creds.Put(resterKey(name), rootKey, b)
}

// PutRester stores a new rester and refuses a name that is already stored, so a second Add never
// silently replaces a rester and its users. UpdateRester is the call that replaces.
func (s *Service) PutRester(rootSecret, name string, r Rester) error {
	s.resterMu.Lock()
	defer s.resterMu.Unlock()
	names, err := s.ListResters()
	if err != nil {
		return err
	}
	if slices.Contains(names, name) {
		return fmt.Errorf("vault: a rester named %q is already stored, edit it instead", name)
	}
	return s.putRester(rootSecret, name, r)
}

// ListResters lists stored rester names only, never a decrypted value, without needing a root secret.
func (s *Service) ListResters() ([]string, error) {
	names, err := s.creds.List()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if rest, ok := strings.CutPrefix(n, resterKeyPrefix); ok {
			out = append(out, rest)
		}
	}
	return out, nil
}

// DeleteRester removes a stored rester. It succeeds even if name was never stored.
func (s *Service) DeleteRester(name string) error {
	return s.creds.Delete(resterKey(name))
}

// GetRester decrypts name's stored rester in full, passwords and tokens included, for a human
// editing it. It is never reachable from the agent or the CLI.
func (s *Service) GetRester(rootSecret, name string) (Rester, error) {
	rootKey, err := DeriveKey(rootSecret, s.rootSalt)
	if err != nil {
		return Rester{}, err
	}
	defer Zero(rootKey)

	b, err := s.creds.Get(resterKey(name), rootKey)
	if err != nil {
		return Rester{}, fmt.Errorf("vault: reading rester %s: %w", name, err)
	}
	defer Zero(b)

	r, err := decodeRester(b)
	if err != nil {
		return Rester{}, fmt.Errorf("vault: decoding rester %s: %w", name, err)
	}
	return r, nil
}

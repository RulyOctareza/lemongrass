// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
)

// Effective is u as it exists in environment env: an override's Fields and FieldTypes replace the
// user's value key by key and inherit the rest, and an override Token replaces the user's token.
// The result has no overrides and lists only env.
func (u ResterUser) Effective(env string) ResterUser {
	out := ResterUser{
		Name:         u.Name,
		Fields:       maps.Clone(u.Fields),
		FieldTypes:   maps.Clone(u.FieldTypes),
		Token:        u.Token,
		Tags:         slices.Clone(u.Tags),
		Environments: []string{env},
	}
	o, ok := u.Overrides[env]
	if !ok {
		return out
	}
	if len(o.Fields) > 0 && out.Fields == nil {
		out.Fields = make(map[string]string, len(o.Fields))
	}
	maps.Copy(out.Fields, o.Fields)
	if len(o.FieldTypes) > 0 && out.FieldTypes == nil {
		out.FieldTypes = make(map[string]string, len(o.FieldTypes))
	}
	maps.Copy(out.FieldTypes, o.FieldTypes)
	if o.Token != "" {
		out.Token = o.Token
	}
	return out
}

// CanUse reports whether user u, assigned to environment envName, can be used by a channel of
// type typ. app needs an App side and a login user carrying every field a fill step names. api needs the environment's BaseURL, an API side, and a token or a login endpoint.
// web needs the environment's Origin, a web side, and a login user carrying every field a fill
// step names, since a bring-your-own-token user has nothing to type into a login page.
func (r Rester) CanUse(typ, envName string, u ResterUser) bool {
	env, ok := r.environment(envName)
	if !ok || !u.assignedTo(envName) {
		return false
	}
	eff := u.Effective(envName)
	switch typ {
	case TypeApp:
		if r.App == nil || eff.IsBYOT() {
			return false
		}
		for _, s := range r.App.Login {
			if s.Action != AppStepFill {
				continue
			}
			if _, ok := eff.Fields[s.Field]; !ok {
				return false
			}
		}
		return true
	case TypeAPI:
		return env.BaseURL != "" && r.API != nil && (eff.IsBYOT() || r.API.LoginEndpoint != "")
	case TypeWeb:
		if env.Origin == "" || r.Web == nil || eff.IsBYOT() {
			return false
		}
		for _, s := range r.Web.Login {
			if s.Action != StepFill {
				continue
			}
			if _, ok := eff.Fields[s.Field]; !ok {
				return false
			}
		}
		return true
	}
	return false
}

// ResolvedRester is a rester flattened for one environment and one channel type: what a channel
// stores as its wrapped copy and what the gates read, so they never see an environment, another
// environment's users, or a user the channel's type cannot use. Its keys are those of the old
// Domain, so a channel wrapped before resters existed decodes as one.
type ResolvedRester struct {
	Environment string
	BaseURL     string
	Origin      string

	LoginEndpoint   string
	TokenPath       string
	TTLOrigin       string
	FixedTTLSeconds int64
	TokenPlacement  TokenPlacement
	Headers         []Header

	Login []LoginStep

	// Package and AppLogin are the app side, set only on an app channel.
	Package  string
	AppLogin []AppStep

	Users []ResterUser
}

// resolve flattens r for envName and typ. It fails when the environment or the side is missing or
// the environment has no address for typ. The result may hold no users; CreateHTTPChannel refuses that.
// The API settings are included on a web channel too, for the token hints the browser's redaction needs.
func (r Rester) resolve(envName, typ string) (ResolvedRester, error) {
	env, ok := r.environment(envName)
	if !ok {
		return ResolvedRester{}, fmt.Errorf("vault: this rester has no environment %q", envName)
	}
	out := ResolvedRester{Environment: envName}
	if r.API != nil {
		out.BaseURL = env.BaseURL
		out.LoginEndpoint = r.API.LoginEndpoint
		out.TokenPath = r.API.TokenPath
		out.TTLOrigin = r.API.TTLOrigin
		out.FixedTTLSeconds = r.API.FixedTTLSeconds
		out.TokenPlacement = r.API.TokenPlacement
		out.Headers = MergeHeaders(r.API.Headers, env.Headers)
	}
	switch typ {
	case TypeAPI:
		if r.API == nil {
			return ResolvedRester{}, fmt.Errorf("vault: this rester has no api side")
		}
		if env.BaseURL == "" {
			return ResolvedRester{}, fmt.Errorf("vault: environment %q has no base URL, so it has no api side", envName)
		}
	case TypeApp:
		if r.App == nil {
			return ResolvedRester{}, fmt.Errorf("vault: this rester has no app side")
		}
		out.Package = r.App.Package
		out.AppLogin = slices.Clone(r.App.Login)
	case TypeWeb:
		if r.Web == nil {
			return ResolvedRester{}, fmt.Errorf("vault: this rester has no web side")
		}
		if env.Origin == "" {
			return ResolvedRester{}, fmt.Errorf("vault: environment %q has no origin, so it has no web side", envName)
		}
		out.Origin = env.Origin
		out.Login = slices.Clone(r.Web.Login)
	default:
		return ResolvedRester{}, fmt.Errorf("vault: unknown channel type %q, want %q, %q or %q", typ, TypeAPI, TypeWeb, TypeApp)
	}
	for _, u := range r.Users {
		if r.CanUse(typ, envName, u) {
			out.Users = append(out.Users, u.Effective(envName))
		}
	}
	return out, nil
}

// MergeHeaders is shared with env's headers laid over it, the environment's winning a name clash.
func MergeHeaders(shared, env []Header) []Header {
	if len(shared) == 0 && len(env) == 0 {
		return nil
	}
	taken := make(map[string]bool, len(env))
	for _, h := range env {
		taken[http.CanonicalHeaderKey(h.Name)] = true
	}
	out := make([]Header, 0, len(shared)+len(env))
	for _, h := range shared {
		if !taken[http.CanonicalHeaderKey(h.Name)] {
			out = append(out, h)
		}
	}
	return append(out, env...)
}

// ReservesHeader reports whether name is set by the proxy on a call: a proxy header or the token header.
func (d ResolvedRester) ReservesHeader(name string) bool {
	return reservesHeader(d.TokenPlacement, name)
}

// SetsHeader reports whether one of the rester's own headers is named name.
func (d ResolvedRester) SetsHeader(name string) bool {
	for _, h := range d.Headers {
		if strings.EqualFold(h.Name, name) {
			return true
		}
	}
	return false
}

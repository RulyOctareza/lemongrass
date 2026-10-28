// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"fmt"

	"github.com/faizalv/lemongrass/internal/vault"
)

// openAPIChannel opens an active channel for an api operation, refusing a web or an app channel.
func (g *Gate) openAPIChannel(id vault.ChannelID) (vault.HTTPChannel, vault.ResolvedRester, error) {
	c, rester, err := g.vault.OpenHTTPChannel(id)
	if err != nil {
		return vault.HTTPChannel{}, vault.ResolvedRester{}, err
	}
	if c.Type == vault.TypeApp {
		return vault.HTTPChannel{}, vault.ResolvedRester{}, fmt.Errorf("restergate: this is an app channel for the %s environment, so it takes only app-login, not api calls", c.Environment)
	}
	if c.Type != vault.TypeAPI {
		return vault.HTTPChannel{}, vault.ResolvedRester{}, fmt.Errorf("restergate: this is a %s channel for the %s environment, so it takes only the web commands (open, user, snapshot, click, type, wait, network, console, screenshot, back, reload, press), not api calls", c.Type, c.Environment)
	}
	return c, rester, nil
}

// WebLoginFields is what the app needs to log one effective user in on a web channel: the
// environment's origin and login steps, the user's field values and the api side's token hints
// the browser uses to redact. It goes only to the app, never to a model.
type WebLoginFields struct {
	User           string               `json:"user"`
	Environment    string               `json:"environment"`
	Origin         string               `json:"origin"`
	BaseURL        string               `json:"base_url"`
	Login          []vault.LoginStep    `json:"login"`
	Fields         map[string]string    `json:"fields"`
	TokenPlacement vault.TokenPlacement `json:"token_placement"`
	TokenPath      string               `json:"token_path"`
}

// WebLoginFields resolves user (a handle, or empty for a channel with one web user) among the
// web channel's users and returns its login material. It fails unless the channel is an active,
// unexpired web channel.
func (g *Gate) WebLoginFields(id vault.ChannelID, user string) (WebLoginFields, error) {
	_, web, err := g.vault.OpenWebChannel(id)
	if err != nil {
		return WebLoginFields{}, err
	}
	name, err := resolveUser(vault.ResolvedRester{Users: web.Users}, user)
	if err != nil {
		return WebLoginFields{}, err
	}
	u, err := findResterUser(vault.ResolvedRester{Users: web.Users}, name)
	if err != nil {
		return WebLoginFields{}, err
	}
	fields := u.Fields
	if fields == nil {
		fields = map[string]string{}
	}
	login := web.Login
	if login == nil {
		login = []vault.LoginStep{}
	}
	return WebLoginFields{
		User:           vault.UserHandle(u.Name),
		Environment:    web.Environment,
		Origin:         web.Origin,
		BaseURL:        web.BaseURL,
		Login:          login,
		Fields:         fields,
		TokenPlacement: web.TokenPlacement,
		TokenPath:      web.TokenPath,
	}, nil
}

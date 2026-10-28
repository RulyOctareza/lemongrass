// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package restergate

import (
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

const (
	HTTPStatusActive   = "active"
	HTTPStatusInactive = "inactive"
	HTTPStatusExpired  = "expired"
)

// HTTPUserInfo is what a model may learn about a rester user: a name and its tags, never a
// login field or a token.
type HTTPUserInfo struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// HTTPChannelInfo is a channel's model-facing summary. BaseURL, Origin and Users need the channel's
// decrypted rester copy, so they are only present while the channel is active.
type HTTPChannelInfo struct {
	Name             string             `json:"name"`
	Rester           string             `json:"rester"`
	Environment      string             `json:"environment"`
	Type             string             `json:"type"`
	Status           string             `json:"status"`
	ExpiresAt        time.Time          `json:"expires_at"`
	RemainingSeconds int64              `json:"remaining_seconds"`
	Methods          []string           `json:"methods"`
	Exclusions       []vault.MethodPath `json:"exclusions"`
	BaseURL          string             `json:"base_url,omitempty"`
	Origin           string             `json:"origin,omitempty"`
	Package          string             `json:"package,omitempty"`
	WebOrigins       []string           `json:"web_origins,omitempty"`
	WebExclusions    []vault.MethodPath `json:"web_exclusions,omitempty"`
	Users            []HTTPUserInfo     `json:"users,omitempty"`
}

// HTTPChannelInfo reports channel id's name, rester, expiry, scope and, while the channel is
// active, its base URL and users.
func (g *Gate) HTTPChannelInfo(id vault.ChannelID) (HTTPChannelInfo, error) {
	c, err := g.vault.HTTPChannelScope(id)
	if err != nil {
		return HTTPChannelInfo{}, err
	}

	now := time.Now()
	info := HTTPChannelInfo{
		Name:        c.Name,
		Rester:      c.Rester,
		Environment: c.Environment,
		Type:        c.Type,
		ExpiresAt:   c.ExpiresAt,
		Methods:     c.Scope.Methods,
		Exclusions:  c.Scope.Exclusions,
	}
	if c.WebScope != nil {
		info.WebOrigins = c.WebScope.Origins
		info.WebExclusions = c.WebScope.Exclusions
	}
	if info.Methods == nil {
		info.Methods = []string{}
	}
	if info.Exclusions == nil {
		info.Exclusions = []vault.MethodPath{}
	}

	if c.Expired(now) {
		info.Status = HTTPStatusExpired
		return info, nil
	}
	info.RemainingSeconds = int64(c.ExpiresAt.Sub(now).Seconds())

	rester, err := g.vault.ChannelResolved(id)
	if err != nil {
		info.Status = HTTPStatusInactive
		return info, nil
	}
	info.Status = HTTPStatusActive
	switch c.Type {
	case vault.TypeWeb:
		info.Origin = rester.Origin
	case vault.TypeApp:
		info.Package = rester.Package
	default:
		info.BaseURL = rester.BaseURL
	}
	info.Users = userInfos(rester)
	return info, nil
}

func userInfos(rester vault.ResolvedRester) []HTTPUserInfo {
	out := make([]HTTPUserInfo, len(rester.Users))
	for i, u := range rester.Users {
		tags := u.Tags
		if tags == nil {
			tags = []string{}
		}
		out[i] = HTTPUserInfo{Name: vault.UserHandle(u.Name), Tags: tags}
	}
	return out
}

// HTTPChannelUsers lists the users of channel id's rester with their tags, under the same
// expiry and active checks as RequestHTTP.
func (g *Gate) HTTPChannelUsers(id vault.ChannelID) ([]HTTPUserInfo, error) {
	_, rester, err := g.vault.OpenHTTPChannel(id)
	if err != nil {
		return nil, err
	}
	return userInfos(rester), nil
}

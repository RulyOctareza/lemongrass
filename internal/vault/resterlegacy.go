// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import "encoding/json"

// legacyDomain is the flat record a domain was stored as before resters had environments.
type legacyDomain struct {
	BaseURL         string
	LoginEndpoint   string
	TokenPath       string
	TTLOrigin       string
	FixedTTLSeconds int64
	TokenPlacement  TokenPlacement
	Users           []ResterUser
	Headers         []Header
}

// decodeRester reads a stored rester. A record with no Environments key is an old domain: it
// becomes a rester with an API side from the domain's settings, one environment named default
// holding its BaseURL, and every user assigned to it. The record keeps its old shape on disk
// until its next save.
func decodeRester(b []byte) (Rester, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return Rester{}, err
	}
	if _, ok := keys["Environments"]; ok {
		var r Rester
		if err := json.Unmarshal(b, &r); err != nil {
			return Rester{}, err
		}
		return r, nil
	}
	var d legacyDomain
	if err := json.Unmarshal(b, &d); err != nil {
		return Rester{}, err
	}
	api := ResterAPI{
		LoginEndpoint:   d.LoginEndpoint,
		TokenPath:       d.TokenPath,
		TTLOrigin:       d.TTLOrigin,
		FixedTTLSeconds: d.FixedTTLSeconds,
		TokenPlacement:  d.TokenPlacement,
		Headers:         d.Headers,
	}
	// A token user kept its token and ignored any login fields before resters, and a rester refuses both.
	for i, u := range d.Users {
		if u.Token != "" {
			d.Users[i].Fields, d.Users[i].FieldTypes = nil, nil
		}
	}
	return NewAPIRester(d.BaseURL, api, d.Users), nil
}

// decodeResolved reads a channel's wrapped copy. An old copy is a whole domain, which is already
// the flat shape a ResolvedRester has, so it decodes as one with its environment reading as default.
func decodeResolved(b []byte) (ResolvedRester, error) {
	var r ResolvedRester
	if err := json.Unmarshal(b, &r); err != nil {
		return ResolvedRester{}, err
	}
	if r.Environment == "" {
		r.Environment = DefaultEnvironment
	}
	return r, nil
}

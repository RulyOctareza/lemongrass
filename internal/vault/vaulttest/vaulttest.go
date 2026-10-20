// Package vaulttest holds fixtures for tests that need a rester without caring about environments.
package vaulttest

import "github.com/faizalv/lemongrass/internal/vault"

// Domain is the flat shape a rester had before environments: one base URL and one set of
// settings. Rester turns it into a rester with one environment named default.
type Domain struct {
	BaseURL         string
	LoginEndpoint   string
	TokenPath       string
	TTLOrigin       string
	FixedTTLSeconds int64
	TokenPlacement  vault.TokenPlacement
	Users           []vault.ResterUser
	Headers         []vault.Header
}

func (d Domain) Rester() vault.Rester {
	return vault.NewAPIRester(d.BaseURL, vault.ResterAPI{
		LoginEndpoint:   d.LoginEndpoint,
		TokenPath:       d.TokenPath,
		TTLOrigin:       d.TTLOrigin,
		FixedTTLSeconds: d.FixedTTLSeconds,
		TokenPlacement:  d.TokenPlacement,
		Headers:         d.Headers,
	}, d.Users)
}

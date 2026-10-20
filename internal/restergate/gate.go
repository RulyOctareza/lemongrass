// Package restergate makes HTTP calls through a channel: it checks scope, logs users in, caches their tokens and sends the request. The rester config comes from the vault and only tokens are cached here.
package restergate

import (
	"sync"
	"time"

	"github.com/faizalv/lemongrass/internal/channelaudit"
	"github.com/faizalv/lemongrass/internal/vault"
)

// RequestTimeout bounds one proxied call end to end at every hop: the caller's socket, both
// daemons' connections and the upstream request. Login and admin operations keep their own deadline.
var RequestTimeout = 120 * time.Second

type Gate struct {
	vault *vault.Service

	// Audit is the shared connector audit log. Nil in a Gate built for a test that doesn't care about it; always set on the Gate a running vault daemon serves.
	Audit *channelaudit.Store

	mu     sync.Mutex
	tokens map[tokenCacheKey]cachedToken
}

// New returns a Gate reading rester config from svc and dropping a channel's cached tokens whenever the vault invalidates that channel.
func New(svc *vault.Service) *Gate {
	g := &Gate{vault: svc, tokens: make(map[tokenCacheKey]cachedToken)}
	svc.OnInvalidate(g.forgetTokens)
	return g
}

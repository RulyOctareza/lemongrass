// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package restergate

import "github.com/faizalv/lemongrass/internal/vault"

// FlushHTTPTokens evicts channel id's cached tokens, for one user or, when user is empty, for
// every user, and returns how many were evicted. The next request for an evicted user logs in
// again. It runs under the same expiry and active checks as RequestHTTP, and a bring-your-own-token
// user's pasted token is put back in the cache by that next request.
func (g *Gate) FlushHTTPTokens(id vault.ChannelID, user string) (int, error) {
	_, rester, err := g.openAPIChannel(id)
	if err != nil {
		return 0, err
	}
	if user != "" {
		if user, err = resolveUser(rester, user); err != nil {
			return 0, err
		}
	}
	return g.flushTokens(id, user), nil
}

func (g *Gate) flushTokens(id vault.ChannelID, user string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	flushed := 0
	for key, t := range g.tokens {
		if key.channel != id || (user != "" && key.user != user) {
			continue
		}
		vault.LockedFree(t.locked)
		delete(g.tokens, key)
		flushed++
	}
	return flushed
}

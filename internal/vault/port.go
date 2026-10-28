// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"errors"
	"fmt"
	"net"
)

// wireProxyPortRangeStart and wireProxyPortRangeEnd bound the local TCP ports a channel's wire
// proxy listener can be assigned, picked above the range most OSes hand out for short-lived
// ephemeral sockets so a channel's port stays stable and doesn't collide with unrelated traffic.
const (
	wireProxyPortRangeStart = 40000
	wireProxyPortRangeEnd   = 40999
)

// ErrNoPortAvailable reports every port in the wire-proxy range is already assigned to a channel.
var ErrNoPortAvailable = errors.New("vault: no wire-proxy port available in range")

// allocatePort picks the lowest port in the wire-proxy range that's neither already assigned to
// another channel nor actually bound by anything else on the machine right now.
func (s *Service) allocatePort() (int, error) {
	existing, err := s.ListChannels()
	if err != nil {
		return 0, err
	}
	used := make(map[int]bool, len(existing))
	for _, c := range existing {
		used[c.Port] = true
	}

	for port := wireProxyPortRangeStart; port <= wireProxyPortRangeEnd; port++ {
		if used[port] {
			continue
		}
		if portAvailable(port) {
			return port, nil
		}
	}
	return 0, ErrNoPortAvailable
}

// portAvailable reports whether port can be bound on loopback right now.
func portAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

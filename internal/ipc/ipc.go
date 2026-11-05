// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package ipc is the local transport between lemongrass processes: unix sockets on Linux and macOS, named pipes on
// Windows, where Electron's Node has no AF_UNIX. Callers pass an address from Address or a filesystem path and never
// branch on the OS themselves.
package ipc

import (
	"net"
	"time"
)

// Listen opens a listener on addr that only the current user can reach.
func Listen(addr string) (net.Listener, error) {
	return listen(addr)
}

// Dial connects to addr, giving up after timeout.
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	return dial(addr, timeout)
}

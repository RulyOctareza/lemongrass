// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package ipc

import "net"

// Peer is the process on the other end of a server-side connection, as the kernel reports it.
type Peer struct {
	PID      int
	SameUser bool
}

// PeerOf reports the connecting process of conn. ok is false where the platform or the connection gives no
// kernel-verified peer, which callers treat as nothing to check.
func PeerOf(conn net.Conn) (peer Peer, ok bool, err error) {
	return peerOf(conn)
}

// PeerExecutable is the full path of the binary process pid runs.
func PeerExecutable(pid int) (string, error) {
	return peerExecutable(pid)
}

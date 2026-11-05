// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !linux && !windows

package ipc

import (
	"errors"
	"net"
)

// No peer credentials are read here yet; macOS peer checks are their own task.
func peerOf(conn net.Conn) (Peer, bool, error) {
	return Peer{}, false, nil
}

func peerExecutable(pid int) (string, error) {
	return "", errors.New("ipc: peer executable not supported on this platform")
}

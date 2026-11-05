// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/faizalv/lemongrass/internal/ipc"
)

// VerifyPeerUID checks that conn's connecting process runs as this process's user, from the kernel's record of the
// peer (SO_PEERCRED, or the named pipe's client process token on Windows). A platform without one passes.
func VerifyPeerUID(conn net.Conn) error {
	peer, ok, err := ipc.PeerOf(conn)
	if err != nil {
		return fmt.Errorf("vault: reading peer credentials: %w", err)
	}
	if ok && !peer.SameUser {
		return errors.New("vault: connection rejected: peer uid mismatch")
	}
	return nil
}

// VerifyPeerBinary checks that conn's connecting process is running this same lgrassd binary. Scoped to the ops the
// agent process actually issues (see requiresPeerBinaryCheck in ipc.go); Electron's own binary path isn't fixed yet,
// so its ops stay at user-only verification.
func VerifyPeerBinary(conn net.Conn) error {
	peer, ok, err := ipc.PeerOf(conn)
	if err != nil {
		return fmt.Errorf("vault: reading peer credentials: %w", err)
	}
	if !ok {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("vault: resolving own binary path: %w", err)
	}
	peerExe, err := ipc.PeerExecutable(peer.PID)
	if err != nil {
		return fmt.Errorf("vault: resolving peer binary path: %w", err)
	}
	if !ipc.SameExecutable(peerExe, self) {
		return errors.New("vault: connection rejected: peer binary mismatch")
	}
	return nil
}

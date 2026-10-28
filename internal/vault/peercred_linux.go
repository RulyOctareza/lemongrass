// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build linux

package vault

import (
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// VerifyPeerUID checks that conn's connecting process shares this process's UID, via the
// kernel-verified SO_PEERCRED credential on the unix socket -- not just the socket file's own
// permission bits, which any process able to open the socket would already satisfy.
func VerifyPeerUID(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil
	}
	cred, err := peerCred(uc)
	if err != nil {
		return fmt.Errorf("vault: reading peer credentials: %w", err)
	}
	if cred.Uid != uint32(os.Getuid()) {
		return errors.New("vault: connection rejected: peer uid mismatch")
	}
	return nil
}

// VerifyPeerBinary checks that conn's connecting process is running this same lgrass binary,
// via /proc/<pid>/exe -- SO_PEERCRED only gives a pid, so the executable path check is a
// separate step. Scoped to the ops the agent process actually issues (see
// requiresPeerBinaryCheck in ipc.go); Electron's own binary path isn't fixed yet, so its ops
// stay at UID-only verification.
func VerifyPeerBinary(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil
	}
	cred, err := peerCred(uc)
	if err != nil {
		return fmt.Errorf("vault: reading peer credentials: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("vault: resolving own binary path: %w", err)
	}
	peerExe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", cred.Pid))
	if err != nil {
		return fmt.Errorf("vault: resolving peer binary path: %w", err)
	}
	if peerExe != self {
		return errors.New("vault: connection rejected: peer binary mismatch")
	}
	return nil
}

func peerCred(uc *net.UnixConn) (*unix.Ucred, error) {
	raw, err := uc.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *unix.Ucred
	var opErr error
	if err := raw.Control(func(fd uintptr) {
		cred, opErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	if opErr != nil {
		return nil, opErr
	}
	return cred, nil
}

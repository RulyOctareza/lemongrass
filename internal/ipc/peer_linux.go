// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build linux

package ipc

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// SO_PEERCRED is the kernel's own record of who connected, not the socket file's permission bits, which any process
// able to open the socket would already satisfy.
func peerOf(conn net.Conn) (Peer, bool, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return Peer{}, false, nil
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return Peer{}, false, err
	}
	var cred *unix.Ucred
	var opErr error
	if err := raw.Control(func(fd uintptr) {
		cred, opErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return Peer{}, false, err
	}
	if opErr != nil {
		return Peer{}, false, opErr
	}
	return Peer{PID: int(cred.Pid), SameUser: cred.Uid == uint32(os.Getuid())}, true, nil
}

func peerExecutable(pid int) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build unix

package dbgate

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// socketPair returns a connected unix stream pair: a net.Conn for the driver and a file to hand
// to ssh. Both ends are close-on-exec, set under ForkLock so a concurrent exec cannot inherit them.
func socketPair() (net.Conn, *os.File, error) {
	syscall.ForkLock.RLock()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(fds[0])
		syscall.CloseOnExec(fds[1])
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, nil, fmt.Errorf("dbgate: creating socket pair for ssh: %w", err)
	}

	localFile := os.NewFile(uintptr(fds[0]), "lgrass-ssh-local")
	remote := os.NewFile(uintptr(fds[1]), "lgrass-ssh-remote")
	local, err := net.FileConn(localFile)
	localFile.Close()
	if err != nil {
		remote.Close()
		return nil, nil, fmt.Errorf("dbgate: creating socket pair for ssh: %w", err)
	}
	return local, remote, nil
}

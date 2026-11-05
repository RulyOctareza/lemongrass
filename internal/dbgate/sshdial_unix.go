// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build unix

package dbgate

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
)

// wireSSH gives ssh one end of a socketpair as its stdin and stdout and returns the other; a
// socket rather than pipes gives the driver its deadlines. release closes this process's copy of
// ssh's end once ssh has started.
func wireSSH(cmd *exec.Cmd) (net.Conn, func(), error) {
	local, remote, err := socketPair()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdin = remote
	cmd.Stdout = remote
	return local, func() { remote.Close() }, nil
}

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

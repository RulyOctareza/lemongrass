// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package dbgate

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
)

// Windows has no socketpair a child can use as its stdio: Go's sockets are overlapped, which a
// console program reading its stdin synchronously cannot use, and one duplex pipe would serialize
// ssh's read and write on a single handle. So ssh gets two plain pipes, and they are bridged into
// a net.Pipe, whose end the driver gets with the deadlines it needs.
func wireSSH(cmd *exec.Cmd) (net.Conn, func(), error) {
	sshIn, toSSH, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("dbgate: creating pipes for ssh: %w", err)
	}
	fromSSH, sshOut, err := os.Pipe()
	if err != nil {
		sshIn.Close()
		toSSH.Close()
		return nil, nil, fmt.Errorf("dbgate: creating pipes for ssh: %w", err)
	}
	cmd.Stdin = sshIn
	cmd.Stdout = sshOut
	local, bridge := net.Pipe()
	go func() {
		_, _ = io.Copy(toSSH, bridge)
		toSSH.Close()
	}()
	go func() {
		_, _ = io.Copy(bridge, fromSSH)
		fromSSH.Close()
		bridge.Close()
	}()
	// ssh's ends close here once it has started, or failed to, so fromSSH ends when ssh exits.
	release := func() {
		sshIn.Close()
		sshOut.Close()
	}
	return local, release, nil
}

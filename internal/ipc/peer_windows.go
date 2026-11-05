// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package ipc

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/sys/windows"

	"github.com/faizalv/lemongrass/internal/privfs"
)

func peerOf(conn net.Conn) (Peer, bool, error) {
	if c, ok := conn.(*greetedConn); ok {
		conn = c.Conn
	}
	h, err := pipeHandle(conn)
	if err != nil {
		return Peer{}, false, nil
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(h, &pid); err != nil {
		return Peer{}, false, fmt.Errorf("ipc: reading the pipe client: %w", err)
	}
	same, err := sameUser(pid)
	if err != nil {
		return Peer{}, false, err
	}
	return Peer{PID: int(pid), SameUser: same}, true, nil
}

func pipeHandle(conn net.Conn) (windows.Handle, error) {
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("ipc: connection is not a named pipe")
	}
	return windows.Handle(f.Fd()), nil
}

// sameUser compares the token user of process pid with ours. A process this user cannot even query counts as
// another user's.
func sameUser(pid uint32) (bool, error) {
	sid, err := privfs.UserSID()
	if err != nil {
		return false, err
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false, nil
	}
	defer windows.CloseHandle(proc)
	var token windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY, &token); err != nil {
		return false, nil
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return false, nil
	}
	return user.User.Sid.String() == sid, nil
}

func peerExecutable(pid int) (string, error) {
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(proc)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(proc, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
)

// Address is the unix socket for name under ~/.lemongrass.
func Address(name string) string {
	return filepath.Join(config.Dir(), name+".sock")
}

// The app probes before starting a daemon, so a socket file still here belongs to a run that stopped uncleanly.
func listen(addr string) (net.Listener, error) {
	os.Remove(addr)
	l, err := net.Listen("unix", addr)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(addr, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

func dial(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", addr, timeout)
}

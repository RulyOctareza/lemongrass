// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package main

import (
	"context"
	"errors"

	"golang.org/x/sys/windows"

	"github.com/faizalv/lemongrass/internal/atomicfile"
)

// Windows delivers no SIGTERM, and ending a task or process is a hard kill, so install and uninstall stop the keeper
// through a named event instead. Local\ scopes both names to this logon session.
const (
	instanceMutexName = `Local\lemongrass-lgrassconf`
	stopEventName     = `Local\lemongrass-lgrassconf-stop`
)

var errKeeperRunning = errors.New("another keeper is already running")

// keeperInstance makes this process the session's only keeper and returns a context that ends when the stop event is
// set. The mutex is held until the process exits.
func keeperInstance(ctx context.Context, home string) (context.Context, error) {
	name, err := windows.UTF16PtrFromString(instanceMutexName)
	if err != nil {
		return nil, err
	}
	mutex, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(mutex)
		return nil, errKeeperRunning
	}
	if err != nil {
		return nil, err
	}
	stopName, err := windows.UTF16PtrFromString(stopEventName)
	if err != nil {
		return nil, err
	}
	stop, err := windows.CreateEvent(nil, 0, 0, stopName)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		windows.WaitForSingleObject(stop, windows.INFINITE)
		cancel()
	}()
	if self, err := resolveBinary(); err == nil {
		if sum, err := fileSHA256(self); err == nil {
			_ = atomicfile.Write(keeperStampPath(home), []byte(sum+"\n"), 0o600)
		}
	}
	return ctx, nil
}

func keeperRunning() bool {
	name, err := windows.UTF16PtrFromString(instanceMutexName)
	if err != nil {
		return false
	}
	mutex, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	windows.CloseHandle(mutex)
	return true
}

func signalKeeperStop() error {
	name, err := windows.UTF16PtrFromString(stopEventName)
	if err != nil {
		return err
	}
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)
	return windows.SetEvent(event)
}

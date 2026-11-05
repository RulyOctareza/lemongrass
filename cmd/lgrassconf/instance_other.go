// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package main

import (
	"context"
	"errors"
)

// systemd and launchd already keep one keeper per user and stop it with SIGTERM, so there is nothing to hold here.
var errKeeperRunning = errors.New("another keeper is already running")

func keeperInstance(ctx context.Context, home string) (context.Context, error) {
	_ = home
	return ctx, nil
}

func attachParentConsole() {}

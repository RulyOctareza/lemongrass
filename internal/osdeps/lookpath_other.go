// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package osdeps

import "os/exec"

var lookPath = exec.LookPath

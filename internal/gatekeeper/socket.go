// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"github.com/faizalv/lemongrass/internal/ipc"
)

func SocketPath() string {
	return ipc.Address("vault")
}

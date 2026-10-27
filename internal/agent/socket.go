// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package agent

import (
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/config"
)

func SocketPath() string {
	return filepath.Join(config.Dir(), "agent.sock")
}

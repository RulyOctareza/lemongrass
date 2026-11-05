// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package hook

import (
	"encoding/json"

	"github.com/faizalv/lemongrass/internal/guard"
)

// shellToolCommand returns the shell text the gate judges for a Bash call, and isShell false for any other tool, whose paths are judged instead. A verdict, never set off Windows, denies the call without judging it.
func shellToolCommand(payload hookEvent) (command string, verdict *guard.Verdict, isShell bool) {
	if payload.ToolName != "Bash" {
		return "", nil, false
	}
	var shell shellToolInput
	if err := json.Unmarshal(payload.ToolInput, &shell); err != nil {
		return "", nil, true
	}
	return shell.Command, nil, true
}

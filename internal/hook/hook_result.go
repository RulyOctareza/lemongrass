// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package hook

import "strings"

type hookResult struct {
	EventName                string
	PermissionDecision       string
	PermissionDecisionReason string
	AdditionalContext        string
}

func newHookResult(eventName, permissionDecision string, parts []string) hookResult {
	var nonEmpty []string
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	context := strings.Join(nonEmpty, "\n\n")
	result := hookResult{
		EventName:          eventName,
		PermissionDecision: permissionDecision,
		AdditionalContext:  context,
	}
	if permissionDecision == "deny" {
		result.PermissionDecisionReason = context
	}
	return result
}

func (r hookResult) hasOutput() bool {
	return r.PermissionDecision == "deny" || r.AdditionalContext != ""
}

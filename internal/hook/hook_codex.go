// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package hook

import (
	"encoding/json"
	"os"
)

type codexHookAdapter struct{}

type codexHookOutput struct {
	HookSpecificOutput codexHookSpecificOutput `json:"hookSpecificOutput"`
}

type codexHookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

func (codexHookAdapter) decode(raw []byte) (hookEvent, error) {
	var event hookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return hookEvent{}, err
	}
	event.PreserveSessionState = true
	return event, nil
}

func (codexHookAdapter) emit(result hookResult) {
	if !result.hasOutput() {
		return
	}
	json.NewEncoder(os.Stdout).Encode(codexHookOutput{HookSpecificOutput: codexHookSpecificOutput{
		HookEventName:            result.EventName,
		PermissionDecision:       result.PermissionDecision,
		PermissionDecisionReason: result.PermissionDecisionReason,
		AdditionalContext:        result.AdditionalContext,
	}})
}

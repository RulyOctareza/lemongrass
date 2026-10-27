// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package hook

import (
	"encoding/json"
	"os"
)

type claudeHookAdapter struct{}

type claudeHookOutput struct {
	HookSpecificOutput claudeHookSpecificOutput `json:"hookSpecificOutput"`
}

type claudeHookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

func (claudeHookAdapter) decode(raw []byte) (hookEvent, error) {
	var event hookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return hookEvent{}, err
	}
	event.EnforceBibliothek = true
	return event, nil
}

func (claudeHookAdapter) emit(result hookResult) {
	if !result.hasOutput() {
		return
	}
	context := result.AdditionalContext
	if result.PermissionDecision == "deny" {
		context = ""
	}
	json.NewEncoder(os.Stdout).Encode(claudeHookOutput{HookSpecificOutput: claudeHookSpecificOutput{
		HookEventName:            result.EventName,
		PermissionDecision:       result.PermissionDecision,
		PermissionDecisionReason: result.PermissionDecisionReason,
		AdditionalContext:        context,
	}})
}

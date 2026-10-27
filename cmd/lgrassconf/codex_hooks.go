// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"fmt"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

var codexHookEvents = []string{"SessionStart", "SessionEnd", "PreToolUse", "PostToolUse", "UserPromptSubmit", "Stop", "PermissionRequest", "Interrupt"}

func reconcileCodexHooks(data []byte, lgrassdPath string) ([]byte, bool, error) {
	registrations := make([]agentconf.HookRegistration, 0, len(codexHookEvents))
	for _, event := range codexHookEvents {
		handler := agentconf.HookHandler{
			Type:    "command",
			Command: fmt.Sprintf("LGRASS_HOOK_VENDOR=codex %s hook %s", lgrassdPath, event),
		}
		matcher := ""
		switch event {
		case "SessionStart":
			matcher = "startup|resume|clear|compact"
			handler.AdditionalContextLimit = 5000
		case "SessionEnd", "Interrupt":
			handler.Timeout = 3
		}
		registrations = append(registrations, agentconf.HookRegistration{
			Event:   event,
			Matcher: matcher,
			Handler: handler,
		})
	}
	return agentconf.ReconcileHookGroups(data, registrations)
}

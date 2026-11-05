// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// The fake hook binary is a sh script, which Windows cannot execute.

//go:build !windows

package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

func fakeHookBinary(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lgrassd")
	script := "#!/bin/sh\ncat >/dev/null\n[ \"$" + replayEnv + "\" = 1 ] || exit 3\nprintf '%s' '" + output + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRecheckHeldCallPassesDenyReasonThrough(t *testing.T) {
	deny := fakeHookBinary(t, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"lgrass: blocked."}}`)
	reason, err := RecheckHeldCall(session.HeldCall{ID: "abc", HookBinary: deny, Payload: []byte(`{}`)})
	if err != nil || reason != "lgrass: blocked." {
		t.Errorf("RecheckHeldCall = %q, %v, want the deny reason", reason, err)
	}
	allow := fakeHookBinary(t, "")
	reason, err = RecheckHeldCall(session.HeldCall{ID: "abc", HookBinary: allow, Payload: []byte(`{}`)})
	if err != nil || reason != "" {
		t.Errorf("RecheckHeldCall = %q, %v, want a pass", reason, err)
	}
}

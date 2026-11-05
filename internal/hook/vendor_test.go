// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import "testing"

func TestHookVendorFromArgumentOrEnvironment(t *testing.T) {
	t.Setenv("LGRASS_HOOK_VENDOR", "")
	if got := hookVendor([]string{"--vendor", "codex"}); got != "codex" {
		t.Errorf("argument form: %q", got)
	}
	if got := hookVendor(nil); got != "" {
		t.Errorf("no vendor: %q", got)
	}
	t.Setenv("LGRASS_HOOK_VENDOR", "codex")
	if got := hookVendor(nil); got != "codex" {
		t.Errorf("environment fallback: %q", got)
	}
	if _, ok := hookAdapterFor("codex").(codexHookAdapter); !ok {
		t.Error("codex vendor does not pick the Codex adapter")
	}
	if _, ok := hookAdapterFor("").(claudeHookAdapter); !ok {
		t.Error("no vendor does not pick the Claude adapter")
	}
}

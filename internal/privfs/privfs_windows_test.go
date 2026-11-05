// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package privfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// A file created inside a restricted directory carries only the user's entry, inherited.
func TestRestrictedDirectoryPassesItsDACLOn(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "home")
	if err := MkdirAll(dir); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "store")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sid, err := UserSID()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dir, file} {
		sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		sddl := sd.String()
		if strings.Count(sddl, "(") != 1 || !strings.Contains(sddl, sid) {
			t.Fatalf("%s: DACL %s, want one entry for %s", p, sddl, sid)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package privfs

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

// UserSID is the current process's user SID in string form, such as S-1-5-21-....
var UserSID = sync.OnceValues(func() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("privfs: reading the process user: %w", err)
	}
	return user.User.Sid.String(), nil
})

// Restrict replaces path's DACL with one protected entry granting the current user full control. A directory's entry
// is inheritable, so files created inside later, including temp files renamed into place, get the same DACL; the
// protected flag stops the profile's inherited entries (SYSTEM, Administrators) from reaching it.
func Restrict(path string) error {
	sid, err := UserSID()
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("privfs: %w", err)
	}
	inherit := ""
	if info.IsDir() {
		inherit = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + inherit + ";FA;;;" + sid + ")")
	if err != nil {
		return fmt.Errorf("privfs: building the DACL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("privfs: reading the DACL: %w", err)
	}
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	if err != nil {
		return fmt.Errorf("privfs: restricting %s: %w", path, err)
	}
	return nil
}

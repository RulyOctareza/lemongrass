// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// renameWindow is how long Rename keeps retrying. Readers such as editors, indexers and antivirus scanners open a file
// briefly without FILE_SHARE_DELETE, and while they hold it a rename over it fails.
const renameWindow = time.Second

// Rename moves oldpath over newpath, retrying while another process holds newpath open without delete sharing.
func Rename(oldpath, newpath string) error {
	deadline := time.Now().Add(renameWindow)
	wait := 10 * time.Millisecond
	for {
		err := os.Rename(oldpath, newpath)
		if err == nil || !transient(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(wait)
		if wait < 100*time.Millisecond {
			wait *= 2
		}
	}
}

func transient(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

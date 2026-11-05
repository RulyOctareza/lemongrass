// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/faizalv/lemongrass/internal/privfs"
)

// processRights is everything another process of this user may still open the vault with: query limited
// information, wait on it, and terminate it, which the app needs to stop it. No memory read or write, no thread
// creation, no handle duplication, so nothing can read the keys or inject code.
const processRights = windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE | windows.PROCESS_TERMINATE

// hardenProcess is the Windows counterpart of PR_SET_DUMPABLE(0). Processes of the same user can by default open
// each other with full access, so the process DACL is replaced by one that grants only processRights, also to OWNER
// RIGHTS, which otherwise keeps the owner's implicit right to rewrite the DACL. WER is told to leave this binary
// alone so a crash writes no dump of the vault's memory. Best-effort: it logs rather than fails startup.
func hardenProcess() {
	if err := restrictProcessDACL(); err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd vault: warning: restricting the process DACL failed: %v\n", err)
	}
	windows.SetErrorMode(windows.SEM_FAILCRITICALERRORS | windows.SEM_NOGPFAULTERRORBOX)
	if err := excludeFromWER(); err != nil {
		fmt.Fprintf(os.Stderr, "lgrassd vault: warning: excluding from error reporting failed: %v\n", err)
	}
}

func restrictProcessDACL() error {
	sid, err := privfs.UserSID()
	if err != nil {
		return err
	}
	rights := fmt.Sprintf("0x%x", processRights)
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;" + rights + ";;;" + sid + ")(A;;" + rights + ";;;OW)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(windows.CurrentProcess(), windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

var procWerAddExcludedApplication = windows.NewLazySystemDLL("wer.dll").NewProc("WerAddExcludedApplication")

// The exclusion is per user and per file name, so it also covers the agent and node daemons, which share the binary.
func excludeFromWER() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(filepath.Base(exe))
	if err != nil {
		return err
	}
	if err := procWerAddExcludedApplication.Find(); err != nil {
		return err
	}
	hr, _, _ := procWerAddExcludedApplication.Call(uintptr(unsafe.Pointer(name)), 0)
	if hr != 0 {
		return fmt.Errorf("WerAddExcludedApplication: HRESULT 0x%x", hr)
	}
	return nil
}

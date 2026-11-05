// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// lgrassconf.exe is linked as a GUI program (-H=windowsgui in the Makefile), so the logon task and the app's launch
// open no console window for the keeper. A GUI program gets no console of its own, so when it is run from a terminal
// it attaches to that terminal's console here and `install` and `uninstall` still print. Output the parent captures
// through pipes is left alone. cmd.exe does not wait for a GUI program, so its prompt can return before the output.
func attachParentConsole() {
	if os.Stdout != nil {
		if kind, err := windows.GetFileType(windows.Handle(os.Stdout.Fd())); err == nil && kind != windows.FILE_TYPE_UNKNOWN {
			return
		}
	}
	const attachParentProcess = ^uintptr(0)
	if ok, _, _ := attachConsole.Call(attachParentProcess); ok == 0 {
		return
	}
	out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	os.Stdout, os.Stderr = out, out
}

var attachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

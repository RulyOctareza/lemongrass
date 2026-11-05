// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package flutter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole     = kernel32.NewProc("AttachConsole")
	procFreeConsole       = kernel32.NewProc("FreeConsole")
	procSetConsoleHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

// scrcpy finishes its mp4 only on Ctrl+C, and Windows delivers Ctrl+C only to every process on one console. So
// scrcpy gets a console of its own, hidden, and nothing else ever shares it; the caller's console holds the agent
// itself and must never receive one.
func recorderProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
}

// interrupt runs lgrass flutter _interrupt in a process with no console, since attaching to scrcpy's console means
// leaving this one, which the calling lgrass still writes to.
func interrupt(p *os.Process) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "flutter", "_interrupt", strconv.Itoa(p.Pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("flutter: interrupting the recording: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Interrupt sends Ctrl+C to the console of the scrcpy process pid. It refuses any other program, so the helper
// cannot be pointed at an arbitrary process.
func Interrupt(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("flutter: opening process %d: %w", pid, err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	err = windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
	windows.CloseHandle(h)
	if err != nil {
		return fmt.Errorf("flutter: reading process %d: %w", pid, err)
	}
	if name := filepath.Base(windows.UTF16ToString(buf[:size])); !strings.EqualFold(name, "scrcpy.exe") {
		return fmt.Errorf("flutter: process %d is %s, not scrcpy", pid, name)
	}
	_, _, _ = procFreeConsole.Call()
	if ok, _, err := procAttachConsole.Call(uintptr(pid)); ok == 0 {
		return fmt.Errorf("flutter: attaching to the console of process %d: %w", pid, err)
	}
	// This helper is on the console too and would end on its own Ctrl+C before reporting back.
	_, _, _ = procSetConsoleHandler.Call(0, 1)
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0); err != nil {
		return fmt.Errorf("flutter: sending Ctrl+C to process %d: %w", pid, err)
	}
	_, _, _ = procFreeConsole.Call()
	return nil
}

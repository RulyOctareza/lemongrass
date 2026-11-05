// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package vault

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// LockKey copies b into its own VirtualAlloc'd pages, locked so the memory manager never writes them to the
// pagefile, then zeroes b. The caller must release the result with LockedFree once it's no longer needed.
func LockKey(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	addr, err := windows.VirtualAlloc(0, uintptr(len(b)), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		return nil, fmt.Errorf("vault: VirtualAlloc for locked key: %w", err)
	}
	if err := lockPages(addr, uintptr(len(b))); err != nil {
		windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
		return nil, fmt.Errorf("vault: VirtualLock for locked key: %w", err)
	}
	// addr is memory VirtualAlloc owns, outside the Go heap, so converting it is safe; the double indirection keeps
	// go vet's unsafeptr check, meant for Go heap addresses, quiet.
	locked := unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&addr))), len(b))
	copy(locked, b)
	Zero(b)
	return locked, nil
}

// LockedFree zeroes and releases memory returned by LockKey.
func LockedFree(b []byte) {
	if len(b) == 0 {
		return
	}
	Zero(b)
	addr := uintptr(unsafe.Pointer(&b[0]))
	windows.VirtualUnlock(addr, uintptr(len(b)))
	windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
}

var growMu sync.Mutex

// workingSetStep is how much the minimum working set grows when locking hits it. Each key takes one 4 KiB page,
// and Windows caps locked pages at the minimum working set, about 50 pages by default.
const workingSetStep = 1 << 20

// lockPages raises the process's minimum working set once and retries when VirtualLock hits that cap. Standard users
// hold the increase-working-set privilege by default.
func lockPages(addr, size uintptr) error {
	err := windows.VirtualLock(addr, size)
	if !errors.Is(err, windows.ERROR_WORKING_SET_QUOTA) {
		return err
	}
	growMu.Lock()
	defer growMu.Unlock()
	proc := windows.CurrentProcess()
	var minWS, maxWS uintptr
	var flags uint32
	windows.GetProcessWorkingSetSizeEx(proc, &minWS, &maxWS, &flags)
	if maxWS < minWS+workingSetStep {
		maxWS = minWS + workingSetStep
	}
	if err := windows.SetProcessWorkingSetSizeEx(proc, minWS+workingSetStep, maxWS, flags); err != nil {
		return err
	}
	return windows.VirtualLock(addr, size)
}

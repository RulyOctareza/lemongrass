// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/faizalv/lemongrass/internal/atomicfile"
)

const environmentKey = `Environment`

// pathMarkerPath records the folder lemongrass added to the user's PATH. ~/.local/bin is shared with tools such as uv
// and pipx, so uninstall removes the entry only when lemongrass is the one that added it.
func pathMarkerPath(home string) string {
	return filepath.Join(home, ".lemongrass", "path-added")
}

// addUserPath appends dir to the user's PATH in HKCU\Environment when no entry names it yet. The marker is written
// before the registry, so a crash between the two can only leave an entry uninstall still removes.
func addUserPath(home, dir string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, environmentKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	current, kind, err := key.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		current, kind, err = "", registry.EXPAND_SZ, nil
	}
	if err != nil {
		return err
	}
	if pathListHas(current, dir) {
		return nil
	}
	if err := atomicfile.Write(pathMarkerPath(home), []byte(dir+"\n"), 0o600); err != nil {
		return err
	}
	if err := setPathValue(key, kind, appendPathEntry(current, dir)); err != nil {
		return err
	}
	broadcastEnvironmentChange()
	return nil
}

func removeUserPath(home string) error {
	marker, err := os.ReadFile(pathMarkerPath(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := strings.TrimSpace(string(marker))
	key, err := registry.OpenKey(registry.CURRENT_USER, environmentKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	current, kind, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	if kept, removed := removePathEntry(current, dir); removed {
		if kept == "" {
			err = key.DeleteValue("Path")
		} else {
			err = setPathValue(key, kind, kept)
		}
		if err != nil {
			return err
		}
		broadcastEnvironmentChange()
	}
	return os.Remove(pathMarkerPath(home))
}

// setPathValue keeps the value's own type, so a REG_EXPAND_SZ PATH holding %VAR% entries stays expandable.
func setPathValue(key registry.Key, kind uint32, value string) error {
	if kind == registry.SZ {
		return key.SetStringValue("Path", value)
	}
	return key.SetExpandStringValue("Path", value)
}

func samePathEntry(entry, dir string) bool {
	if expanded, err := registry.ExpandString(entry); err == nil {
		entry = expanded
	}
	clean := func(p string) string { return strings.TrimRight(filepath.Clean(strings.TrimSpace(p)), `\/`) }
	return entry != "" && strings.EqualFold(clean(entry), clean(dir))
}

func pathListHas(list, dir string) bool {
	for _, entry := range strings.Split(list, ";") {
		if samePathEntry(entry, dir) {
			return true
		}
	}
	return false
}

func appendPathEntry(list, dir string) string {
	list = strings.TrimRight(list, ";")
	if list == "" {
		return dir
	}
	return list + ";" + dir
}

func removePathEntry(list, dir string) (string, bool) {
	var kept []string
	removed := false
	for _, entry := range strings.Split(list, ";") {
		if samePathEntry(entry, dir) {
			removed = true
			continue
		}
		if entry != "" {
			kept = append(kept, entry)
		}
	}
	return strings.Join(kept, ";"), removed
}

// broadcastEnvironmentChange tells Explorer and other top-level windows that the user environment changed, so a
// terminal opened from them afterwards sees the new PATH. Processes already running keep their old one.
func broadcastEnvironmentChange() {
	const (
		hwndBroadcast    = 0xFFFF
		wmSettingChange  = 0x001A
		smtoAbortIfHung  = 0x0002
		broadcastTimeout = 5000
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	sendMessageTimeout.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung,
		broadcastTimeout, uintptr(unsafe.Pointer(&result)))
}

var sendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

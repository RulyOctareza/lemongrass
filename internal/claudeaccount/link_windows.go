// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package claudeaccount

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// makeLink creates a directory junction, which unlike a symlink needs neither admin rights nor Developer Mode. A
// junction can only point at an absolute local directory, which the primary config folder always is. It is built
// under a temporary name and renamed into place, because a junction is a directory first and a link only once its
// reparse point is set, and a concurrent reconcile must never see it in between as an empty folder to replace.
func makeLink(src, dst string) error {
	tmp, err := os.MkdirTemp(filepath.Dir(dst), "."+filepath.Base(dst)+"-link-*")
	if err != nil {
		return err
	}
	if err := setJunction(tmp, src); err != nil {
		os.Remove(tmp)
		return &os.LinkError{Op: "junction", Old: src, New: dst, Err: err}
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		if _, statErr := os.Lstat(dst); statErr == nil {
			return &os.LinkError{Op: "junction", Old: src, New: dst, Err: fs.ErrExist}
		}
		return err
	}
	return nil
}

func setJunction(dir, target string) error {
	target = filepath.Clean(target)
	substitute := utf16.Encode([]rune(`\??\` + target))
	printName := utf16.Encode([]rune(target))

	// REPARSE_DATA_BUFFER for IO_REPARSE_TAG_MOUNT_POINT: tag, data length, reserved, then the substitute and print
	// name offsets and lengths in bytes, then both names, each followed by a NUL.
	var path []uint16
	path = append(path, substitute...)
	path = append(path, 0)
	path = append(path, printName...)
	path = append(path, 0)
	dataLength := 8 + 2*len(path)
	buf := make([]byte, 8+dataLength)
	binary.LittleEndian.PutUint32(buf[0:], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buf[4:], uint16(dataLength))
	binary.LittleEndian.PutUint16(buf[8:], 0)
	binary.LittleEndian.PutUint16(buf[10:], uint16(2*len(substitute)))
	binary.LittleEndian.PutUint16(buf[12:], uint16(2*len(substitute)+2))
	binary.LittleEndian.PutUint16(buf[14:], uint16(2*len(printName)))
	for i, u := range path {
		binary.LittleEndian.PutUint16(buf[16+2*i:], u)
	}

	name, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var returned uint32
	return windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &buf[0], uint32(len(buf)), nil, 0, &returned, nil)
}

// isLink counts any reparse point as a link: since Go 1.23 Lstat no longer reports a junction as ModeSymlink, and a
// junction treated as a plain directory would be walked into, reaching the primary's files.
func isLink(info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

// sameLinkTarget tolerates the forms a junction target is read back in: an NT \??\ or \\?\ prefix, a different case
// and a trailing separator all name the same folder.
func sameLinkTarget(target, src string) bool {
	clean := func(p string) string {
		for _, prefix := range []string{`\??\`, `\\?\`} {
			p = strings.TrimPrefix(p, prefix)
		}
		return strings.TrimRight(filepath.Clean(p), `\`)
	}
	return strings.EqualFold(clean(target), clean(src))
}

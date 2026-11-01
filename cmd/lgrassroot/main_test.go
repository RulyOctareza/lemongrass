// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

type fakeRun struct {
	argv   [][]string
	output string
	err    error
}

func (f *fakeRun) machine(osRelease string, euid int) (machine, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return machine{
		euid:          euid,
		readOSRelease: func() ([]byte, error) { return []byte(osRelease), nil },
		exec: func(argv []string, out io.Writer) error {
			f.argv = append(f.argv, argv)
			io.WriteString(out, f.output)
			return f.err
		},
		stdout: &stdout,
		stderr: &stderr,
	}, &stdout, &stderr
}

const fedora = "ID=fedora\nVERSION_ID=43\n"

func TestRefusals(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		osRelease string
		euid      int
	}{
		{"no arguments", nil, fedora, 0},
		{"one argument", []string{"install"}, fedora, 0},
		{"extra argument", []string{"install", "android-tools", "--allowerasing"}, fedora, 0},
		{"extra package", []string{"install", "android-tools", "git"}, fedora, 0},
		{"unknown verb", []string{"remove", "git"}, fedora, 0},
		{"verb as option", []string{"--install", "git"}, fedora, 0},
		{"package outside the allowlist", []string{"install", "nmap"}, fedora, 0},
		{"option as package", []string{"install", "-y"}, fedora, 0},
		{"repo option as package", []string{"install", "--repo=evil"}, fedora, 0},
		{"path as package", []string{"install", "/tmp/evil.rpm"}, fedora, 0},
		{"url as package", []string{"install", "https://example.com/evil.rpm"}, fedora, 0},
		{"package with an option glued on", []string{"install", "git --nogpgcheck"}, fedora, 0},
		{"empty package", []string{"install", ""}, fedora, 0},
		{"family not built yet", []string{"install", "git"}, "ID=debian\nVERSION_ID=12\n", 0},
		{"not root", []string{"install", "git"}, fedora, 1000},
	}
	for _, c := range cases {
		f := &fakeRun{}
		m, _, stderr := f.machine(c.osRelease, c.euid)
		if code := run(c.args, m); code != exitRefused {
			t.Errorf("%s: exit %d, want %d", c.name, code, exitRefused)
		}
		if len(f.argv) != 0 {
			t.Errorf("%s: ran %v", c.name, f.argv)
		}
		if !strings.HasPrefix(stderr.String(), "lgrassroot: ") {
			t.Errorf("%s: stderr %q", c.name, stderr.String())
		}
	}
}

func TestEveryAllowlistedPackageRuns(t *testing.T) {
	for _, pkg := range []string{"git", "git-core", "android-tools", "ffmpeg-free", "ffmpeg"} {
		for _, verb := range []string{"install", "upgrade"} {
			f := &fakeRun{}
			m, _, _ := f.machine(fedora, 0)
			if code := run([]string{verb, pkg}, m); code != exitDone {
				t.Errorf("%s %s: exit %d", verb, pkg, code)
			}
			want := []string{"/usr/bin/dnf", verb, "-y", pkg}
			if len(f.argv) != 1 || !slices.Equal(f.argv[0], want) {
				t.Errorf("%s %s: ran %v, want %v", verb, pkg, f.argv, want)
			}
		}
	}
}

func TestRockyIsFedoraFamily(t *testing.T) {
	f := &fakeRun{}
	m, _, _ := f.machine("ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\n", 0)
	if code := run([]string{"upgrade", "git-core"}, m); code != exitDone {
		t.Errorf("exit %d", code)
	}
}

func TestOutputStreams(t *testing.T) {
	f := &fakeRun{output: "Installing android-tools\nComplete!\n"}
	m, stdout, _ := f.machine(fedora, 0)
	run([]string{"install", "android-tools"}, m)
	if stdout.String() != f.output {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestLockHeld(t *testing.T) {
	f := &fakeRun{output: "Failed to obtain rpm transaction lock. Another transaction is in progress.\n", err: errors.New("exit status 1")}
	m, _, stderr := f.machine(fedora, 0)
	if code := run([]string{"install", "android-tools"}, m); code != exitLocked {
		t.Errorf("exit %d, want %d", code, exitLocked)
	}
	if !strings.Contains(stderr.String(), "another package manager is running") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestLibdnfStateLockHeld(t *testing.T) {
	f := &fakeRun{output: "Failed to obtain lock \"/var/lib/dnf/rpmdb_lock.pid\"\n", err: errors.New("exit status 1")}
	m, _, _ := f.machine(fedora, 0)
	if code := run([]string{"upgrade", "git-core"}, m); code != exitLocked {
		t.Errorf("exit %d, want %d", code, exitLocked)
	}
}

func TestDnfFailure(t *testing.T) {
	f := &fakeRun{output: "No match for argument: android-tools\n", err: errors.New("exit status 1")}
	m, _, stderr := f.machine(fedora, 0)
	if code := run([]string{"install", "android-tools"}, m); code != exitFailed {
		t.Errorf("exit %d, want %d", code, exitFailed)
	}
	if !strings.HasPrefix(stderr.String(), "lgrassroot: ") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestTailBufferKeepsTheEnd(t *testing.T) {
	tail := &tailBuffer{max: 16}
	io.WriteString(tail, strings.Repeat("x", 100))
	io.WriteString(tail, "lock held")
	if !tail.contains([]string{"LOCK HELD"}) || len(tail.buf) != 16 {
		t.Errorf("buf = %q", tail.buf)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// lgrassroot installs or upgrades one allowlisted package as root. The app runs it through pkexec, and the package's
// polkit rule lets an admin user do so without a password, so it trusts nothing the caller controls: it takes exactly
// a verb and a package name from osdeps.HelperPackages, reads no environment, and builds the package manager's
// command line itself from fixed parts.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/faizalv/lemongrass/internal/osdeps"
)

// Exit codes the app words its result from; pkexec itself uses 126 and 127.
const (
	exitDone    = 0
	exitFailed  = 1
	exitRefused = 2
	exitLocked  = 3
)

const dnfPath = "/usr/bin/dnf"

var verbs = map[string]bool{"install": true, "upgrade": true}

// dnf5 fails at once while another dnf or PackageKit holds the rpm lock or its own state lock. librpm's "waiting for
// transaction lock" and dnf4's wait on another pid only block until the lock frees, so they stream and need no code.
var lockTexts = []string{"failed to obtain rpm transaction lock", `failed to obtain lock "`}

type machine struct {
	euid          int
	readOSRelease func() ([]byte, error)
	// exec runs argv with the output streamed to out and returns its error.
	exec   func(argv []string, out io.Writer) error
	stdout io.Writer
	stderr io.Writer
}

func main() {
	os.Exit(run(os.Args[1:], machine{
		euid: os.Geteuid(),
		readOSRelease: func() ([]byte, error) {
			data, err := os.ReadFile("/etc/os-release")
			if err != nil {
				data, err = os.ReadFile("/usr/lib/os-release")
			}
			return data, err
		},
		exec:   execFixed,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}))
}

// execFixed replaces the environment whole, so nothing pkexec passed through reaches the package manager, and gives
// it no stdin, so it can never wait on a question.
func execFixed(argv []string, out io.Writer) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LC_ALL=C", "LANG=C"}
	cmd.Dir = "/"
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

func run(args []string, m machine) int {
	refuse := func(format string, a ...any) int {
		fmt.Fprintf(m.stderr, "lgrassroot: "+format+"\n", a...)
		return exitRefused
	}
	if len(args) != 2 {
		return refuse("takes exactly two arguments, install or upgrade and a package name")
	}
	verb, pkg := args[0], args[1]
	if !verbs[verb] {
		return refuse("%q is not install or upgrade", verb)
	}
	if m.euid != 0 {
		return refuse("must run as root through pkexec")
	}
	data, err := m.readOSRelease()
	if err != nil {
		return refuse("cannot tell which system this is: %v", err)
	}
	family := osdeps.ParseOSRelease(string(data)).Family
	if family != osdeps.FamilyFedora {
		return refuse("installs are not supported on this system yet")
	}
	if !osdeps.HelperAllows(family, pkg) {
		return refuse("%q is not a package lemongrass installs", pkg)
	}

	tail := &tailBuffer{max: 8192}
	err = m.exec([]string{dnfPath, verb, "-y", pkg}, io.MultiWriter(m.stdout, tail))
	if err == nil {
		return exitDone
	}
	if tail.contains(lockTexts) {
		fmt.Fprintln(m.stderr, "lgrassroot: another package manager is running, try again once it finishes")
		return exitLocked
	}
	fmt.Fprintf(m.stderr, "lgrassroot: dnf %s %s failed: %v\n", verb, pkg, err)
	return exitFailed
}

// tailBuffer keeps the last max bytes written to it, enough to find the package manager's closing error.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) contains(texts []string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	lower := bytes.ToLower(t.buf)
	for _, s := range texts {
		if bytes.Contains(lower, []byte(strings.ToLower(s))) {
			return true
		}
	}
	return false
}

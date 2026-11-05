// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package ipc

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

// A raw client, as Node is, reads the greeting first and finds the token the server wrote to disk.
func TestGreetingCarriesTheTokenOnDisk(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "greet.sock")
	l, err := Listen(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		conn, err := l.Accept()
		if err == nil {
			time.Sleep(time.Second)
			conn.Close()
		}
	}()

	timeout := 2 * time.Second
	conn, err := winio.DialPipe(pipeName(addr), &timeout)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	token, err := os.ReadFile(TokenPath(addr))
	if err != nil {
		t.Fatal(err)
	}
	if line != greetingMagic+strings.TrimSpace(string(token))+"\n" {
		t.Fatalf("greeting %q does not carry token %q", line, token)
	}
}

// A second listener on a name already held fails instead of sharing it.
func TestSecondListenerOnTheSameNameFails(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "first.sock")
	l, err := Listen(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if l2, err := Listen(addr); err == nil {
		l2.Close()
		t.Fatal("second listener on a held pipe name succeeded")
	}
}

func TestAddressNamesThePipeByUserAndName(t *testing.T) {
	got := Address("vault")
	if !strings.HasPrefix(got, `\\.\pipe\lemongrass-S-1-`) || !strings.HasSuffix(got, "-vault") {
		t.Fatalf("Address(vault) = %q", got)
	}
	if want := filepath.Join(filepath.Dir(TokenPath(Address("vault"))), "vault.ipckey"); TokenPath(got) != want {
		t.Fatalf("TokenPath = %q, want %q", TokenPath(got), want)
	}
}

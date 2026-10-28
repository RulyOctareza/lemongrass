// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build unix

package dbgate

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeSSHResolveError = "ssh: Could not resolve hostname bastion-x: Name or service not known"

// TestMain lets the test binary stand in for ssh: run with LGRASS_FAKE_SSH set, it records its
// arguments and either forwards stdin and stdout to the -W address or fails the way ssh does.
func TestMain(m *testing.M) {
	if mode := os.Getenv("LGRASS_FAKE_SSH"); mode != "" {
		os.Exit(fakeSSH(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeSSH(mode string, args []string) int {
	if path := os.Getenv("LGRASS_FAKE_SSH_ARGS"); path != "" {
		_ = os.WriteFile(path, []byte(strings.Join(args, "\n")), 0o600)
	}
	if mode == "fail" {
		os.Stderr.WriteString(fakeSSHResolveError + "\r\n")
		return 255
	}
	var addr string
	for i, a := range args {
		if a == "-W" && i+1 < len(args) {
			addr = args[i+1]
		}
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		os.Stderr.WriteString("channel 0: open failed: connect failed: Connection refused\n")
		return 1
	}
	go func() {
		_, _ = io.Copy(conn, os.Stdin)
		conn.(*net.TCPConn).CloseWrite()
	}()
	_, _ = io.Copy(os.Stdout, conn)
	return 0
}

// useFakeSSH points the dialer at the test binary in mode and returns the file its arguments
// are recorded in.
func useFakeSSH(t *testing.T, mode string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	old := sshBinary
	sshBinary = exe
	t.Cleanup(func() { sshBinary = old })
	argsPath := filepath.Join(t.TempDir(), "args")
	t.Setenv("LGRASS_FAKE_SSH", mode)
	t.Setenv("LGRASS_FAKE_SSH_ARGS", argsPath)
	return argsPath
}

func recordedArgs(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fake ssh args: %v", err)
	}
	return strings.Split(string(b), "\n")
}

func echoListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestSplitSSHAlias(t *testing.T) {
	cases := []struct {
		name, in, wantConn, wantAlias string
		wantErr                       bool
	}{
		{name: "none", in: "mysql://u:p@db:3306/app?parseTime=true", wantConn: "mysql://u:p@db:3306/app?parseTime=true"},
		{name: "alias", in: "postgres://u:p@db:5432/app?lgrass_ssh=bastion-prod&sslmode=disable", wantConn: "postgres://u:p@db:5432/app?sslmode=disable", wantAlias: "bastion-prod"},
		{name: "alias only", in: "mysql://u:p@db:3306/app?lgrass_ssh=jump.example_1", wantConn: "mysql://u:p@db:3306/app", wantAlias: "jump.example_1"},
		{name: "option injection", in: "mysql://u:p@db/app?lgrass_ssh=-oProxyCommand%3Dtouch%20x", wantErr: true},
		{name: "empty", in: "mysql://u:p@db/app?lgrass_ssh=", wantErr: true},
		{name: "twice", in: "mysql://u:p@db/app?lgrass_ssh=a&lgrass_ssh=b", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotConn, gotAlias, err := splitSSHAlias(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("splitSSHAlias(%q) succeeded, want an error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitSSHAlias(%q): %v", tc.in, err)
			}
			if gotConn != tc.wantConn || gotAlias != tc.wantAlias {
				t.Errorf("splitSSHAlias(%q) = %q, %q, want %q, %q", tc.in, gotConn, gotAlias, tc.wantConn, tc.wantAlias)
			}
		})
	}
}

func TestDialSSHForwardsThroughTheAlias(t *testing.T) {
	argsPath := useFakeSSH(t, "forward")
	addr := echoListener(t)

	conn, err := dialSSH("bastion-prod", "tcp", addr)
	if err != nil {
		t.Fatalf("dialSSH: %v", err)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil || string(got) != "ping" {
		t.Fatalf("read = %q, %v, want ping", got, err)
	}

	want := []string{"-o", "BatchMode=yes", "-W", addr, "--", "bastion-prod"}
	if args := recordedArgs(t, argsPath); strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("ssh args = %q, want %q", args, want)
	}

	conn.SetReadDeadline(time.Now().Add(-time.Second))
	var netErr net.Error
	if _, err := conn.Read(got); !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("read past the deadline = %v, want a timeout", err)
	}

	conn.Close()
	select {
	case <-conn.done:
	case <-time.After(5 * time.Second):
		t.Fatal("ssh still running after Close")
	}
	if err := conn.failure(); err != nil {
		t.Errorf("failure after Close = %v, want nil", err)
	}
}

func TestDialSSHRefusesNonTCP(t *testing.T) {
	if _, err := dialSSH("bastion-prod", "unix", "/tmp/.s.PGSQL.5432"); err == nil {
		t.Fatal("dialSSH over unix succeeded, want an error")
	}
}

func TestOpenDBReportsSSHFailure(t *testing.T) {
	for _, connString := range []string{
		"mysql://app:pw@db-internal.invalid:3306/app?lgrass_ssh=bastion-x",
		"postgres://app:pw@db-internal.invalid:5432/app?lgrass_ssh=bastion-x",
	} {
		t.Run(connString[:strings.Index(connString, ":")], func(t *testing.T) {
			argsPath := useFakeSSH(t, "fail")
			db, err := openDB(connString)
			if err != nil {
				t.Fatalf("openDB: %v", err)
			}
			defer db.Close()

			err = pingWithTimeout(db)
			if err == nil || !strings.Contains(err.Error(), "dbgate: ssh: "+fakeSSHResolveError) {
				t.Fatalf("ping = %v, want the ssh error", err)
			}
			// The host name reaches ssh unresolved, since only the far end of the tunnel may know it.
			args := recordedArgs(t, argsPath)
			if !strings.HasPrefix(args[3], "db-internal.invalid:") {
				t.Errorf("ssh -W target = %q, want the unresolved host", args[3])
			}
		})
	}
}

func TestOpenDBKeepsTheDriverErrorThroughAHealthyTunnel(t *testing.T) {
	useFakeSSH(t, "forward")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	db, err := openDB("mysql://app:pw@" + ln.Addr().String() + "/app?lgrass_ssh=bastion-prod")
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = db.PingContext(ctx)
	if err == nil || strings.Contains(err.Error(), "dbgate: ssh") {
		t.Fatalf("ping = %v, want the driver's own error", err)
	}
}

func TestOpenDBRejectsAnInvalidAlias(t *testing.T) {
	if _, err := openDB("mysql://app:pw@db:3306/app?lgrass_ssh=-oProxyCommand%3Dx"); err == nil {
		t.Fatal("openDB accepted an alias starting with a dash")
	}
}

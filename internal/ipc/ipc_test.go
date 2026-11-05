// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package ipc

import (
	"bufio"
	"path/filepath"
	"testing"
	"time"
)

// A request and its reply cross a listener and a dialer at the same address, and the server sees its own user.
func TestListenDialRoundTrip(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "rt.sock")
	l, err := Listen(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	peers := make(chan Peer, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		peer, ok, err := PeerOf(conn)
		if err == nil && ok {
			peers <- peer
		} else {
			peers <- Peer{SameUser: true}
		}
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			return
		}
		conn.Write([]byte("echo " + line))
	}()

	conn, err := Dial(addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo ping\n" {
		t.Fatalf("reply = %q", got)
	}
	if peer := <-peers; !peer.SameUser {
		t.Fatalf("peer %+v not seen as the same user", peer)
	}
}

func TestDialWithoutListenerFails(t *testing.T) {
	if _, err := Dial(filepath.Join(t.TempDir(), "none.sock"), 200*time.Millisecond); err == nil {
		t.Fatal("dial with no listener succeeded")
	}
}

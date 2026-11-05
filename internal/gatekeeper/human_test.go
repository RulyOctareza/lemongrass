// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"bytes"
	"encoding/json"
	"github.com/faizalv/lemongrass/internal/ipc"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

func humanPayload(secret, method, path string) humanRequestPayload {
	return humanRequestPayload{RootSecret: secret, Call: restergate.HumanCall{
		Rester: "d", Environment: vault.DefaultEnvironment, User: "alice", Method: method, Path: path,
	}}
}

func TestHumanRequestOverIPCIgnoresChannelScope(t *testing.T) {
	c, _ := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
	})

	// The stack's channel grants GET only; a human call is not bound by it.
	var result restergate.HumanResult
	if err := c.call(opHumanRequest, humanPayload("root-secret", "DELETE", "/orders/1"), &result); err != nil {
		t.Fatalf("human_request: %v", err)
	}
	if result.Status != 200 || result.Body != "DELETE /orders/1 Bearer tok" {
		t.Errorf("result = %+v, want 200 with the token attached", result)
	}

	var flushed flushedPayload
	if err := c.call(opHumanFlush, humanFlushPayload{Rester: "d", Environment: vault.DefaultEnvironment}, &flushed); err != nil || flushed.Flushed != 1 {
		t.Errorf("human_flush = %+v, %v, want one token flushed", flushed, err)
	}

	if err := c.call(opHumanRequest, humanPayload("wrong", "GET", "/x"), &result); err == nil {
		t.Error("human_request with a wrong passphrase succeeded")
	}
}

func TestHumanDownloadOverIPCStreams(t *testing.T) {
	want := patternBytes(3 << 20)
	c, _ := startDownloadStack(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(want)
	})

	conn, err := ipc.Dial(c.SocketPath, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(humanPayload("root-secret", "GET", "/files/a.bin"))
	json.NewEncoder(conn).Encode(request{Op: opHumanDownload, Payload: payload})
	res, err := ReadDownload(conn, 10*time.Second)
	if err != nil {
		t.Fatalf("ReadDownload: %v", err)
	}
	if res.Body == nil {
		t.Fatalf("result = %+v, want a streamed body", res)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil || !bytes.Equal(got, want) || res.Stream.Name != "a.bin" {
		t.Errorf("read %d bytes, name %q, err %v, want %d bytes named a.bin", len(got), res.Stream.Name, err, len(want))
	}
}

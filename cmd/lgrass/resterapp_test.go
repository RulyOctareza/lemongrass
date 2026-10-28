// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
)

func TestParseAppLoginArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    appLoginCommand
		wantErr string
	}{
		{name: "bare", args: []string{"abc", "app-login"}, want: appLoginCommand{shortID: "abc"}},
		{name: "user", args: []string{"abc", "APP-LOGIN", "--user", "alice"}, want: appLoginCommand{shortID: "abc", user: "alice"}},
		{name: "user and device equals", args: []string{"abc", "app-login", "--device=R58M", "--user=alice"}, want: appLoginCommand{shortID: "abc", user: "alice", device: "R58M"}},
		{name: "unknown flag", args: []string{"abc", "app-login", "--password", "x"}, wantErr: "app-login does not take --password"},
		{name: "stray argument", args: []string{"abc", "app-login", "alice"}, wantErr: "takes only --user and --device"},
		{name: "flag without value", args: []string{"abc", "app-login", "--user"}, wantErr: "--user needs a value"},
		{name: "empty value", args: []string{"abc", "app-login", "--device="}, wantErr: "--device needs a value"},
	}
	for _, tt := range tests {
		got, err := parseAppLoginArgs(tt.args)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: err = %v, want %q", tt.name, err, tt.wantErr)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %+v, %v, want %+v", tt.name, got, err, tt.want)
		}
	}
	if isAppLogin([]string{"abc", "get", "/x"}) || isAppLogin([]string{"abc"}) {
		t.Error("isAppLogin matched a command that is not app-login")
	}
}

func TestFormatAppLogin(t *testing.T) {
	if got := formatAppLogin(restergate.AppLoginResult{User: "alice", OK: true}); got != "as alice\nok\n" {
		t.Errorf("ok = %q", got)
	}
	got := formatAppLogin(restergate.AppLoginResult{User: "alice", FailedStep: 3, FailedAction: "fill", Reason: `"Password" not found within 10s`})
	if got != "as alice\nfailed at step 3 (fill): \"Password\" not found within 10s\n" {
		t.Errorf("failed = %q", got)
	}
}

// appStack runs a real agent over a fake vault that reports channelType and answers app_login with reply, returning an agent client and the short id.
func appStack(t *testing.T, channelType string, reply restergate.AppLoginResult) (*agent.Client, string) {
	t.Helper()
	dir := t.TempDir()
	vaultSock := filepath.Join(dir, "vault.sock")
	vl, err := net.Listen("unix", vaultSock)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { vl.Close() })
	go func() {
		for {
			conn, err := vl.Accept()
			if err != nil {
				return
			}
			var req struct {
				Op      string
				Payload json.RawMessage
			}
			json.NewDecoder(conn).Decode(&req)
			switch req.Op {
			case "http_channel_info":
				json.NewEncoder(conn).Encode(map[string]any{"ok": true, "payload": map[string]any{"info": restergate.HTTPChannelInfo{Status: "active", Type: channelType}}})
			case "http_channel_scope":
				json.NewEncoder(conn).Encode(map[string]any{"ok": true})
			case "app_login":
				// The payload names a user and a serial, so echo them into the reply the way the gate would.
				var p struct{ User, Serial string }
				json.Unmarshal(req.Payload, &p)
				r := reply
				if r.User == "" {
					r.User = p.User + "@" + p.Serial
				}
				json.NewEncoder(conn).Encode(map[string]any{"ok": true, "payload": r})
			default:
				json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "unexpected op"})
			}
			conn.Close()
		}
	}()

	svc := agent.NewService(&gatekeeper.Client{SocketPath: vaultSock})
	short, err := svc.RegisterChannel(vault.ChannelID("real-id"))
	if err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	agentSock := filepath.Join(dir, "agent.sock")
	gl, err := net.Listen("unix", agentSock)
	if err != nil {
		t.Fatalf("net.Listen agent: %v", err)
	}
	t.Cleanup(func() { gl.Close() })
	go agent.Serve(svc, gl, vault.NewFailureLimiter(1000, time.Minute, time.Hour))
	return &agent.Client{SocketPath: agentSock}, short
}

func TestExecAppLogin(t *testing.T) {
	client, short := appStack(t, "app", restergate.AppLoginResult{OK: true})
	var out bytes.Buffer
	ok, err := execAppLogin(client, appLoginCommand{shortID: short, user: "alice", device: "R58M"}, &out)
	if err != nil || !ok || out.String() != "as alice@R58M\nok\n" {
		t.Errorf("exec = %v, %v, %q, want ok with the user and device forwarded", ok, err, out.String())
	}

	client, short = appStack(t, "app", restergate.AppLoginResult{User: "alice", FailedStep: 2, FailedAction: "tap", Reason: "not found"})
	out.Reset()
	ok, err = execAppLogin(client, appLoginCommand{shortID: short}, &out)
	if err != nil || ok || out.String() != "as alice\nfailed at step 2 (tap): not found\n" {
		t.Errorf("failed exec = %v, %v, %q, want a failed step printed under its user", ok, err, out.String())
	}

	if _, err := execAppLogin(client, appLoginCommand{shortID: "nope0000"}, &out); err == nil || !strings.Contains(err.Error(), "no such channel") {
		t.Errorf("unknown id: err = %v", err)
	}
}

func TestRefuseAPIOnApp(t *testing.T) {
	client, short := appStack(t, "app", restergate.AppLoginResult{})
	for _, args := range [][]string{{short, "get", "/orders"}, {short, "flush"}} {
		if err := refuseAPIOnWeb(client, args); err == nil || !strings.Contains(err.Error(), "app channel, it takes app-login") {
			t.Errorf("%v: err = %v, want app-login named", args, err)
		}
	}
	for _, args := range [][]string{{short, "info"}, {short, "users"}} {
		if err := refuseAPIOnWeb(client, args); err != nil {
			t.Errorf("%v: err = %v, want info and users allowed on an app channel", args, err)
		}
	}
}

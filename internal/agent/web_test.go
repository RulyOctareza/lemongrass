// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agent

import (
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/web"
)

const webTestRealID = vault.ChannelID("real-web-channel-0123456789")

// fakeInfoVault answers http_channel_info with info, or with infoErr when it is set, so the web op can be tested without building a rester in a real vault.
func fakeInfoVault(t *testing.T, info restergate.HTTPChannelInfo, infoErr string) *gatekeeper.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			var req struct{ Op string }
			json.NewDecoder(conn).Decode(&req)
			resp := map[string]any{"ok": true}
			if infoErr != "" {
				resp = map[string]any{"ok": false, "error": infoErr}
			} else {
				resp["payload"] = map[string]any{"info": info}
			}
			json.NewEncoder(conn).Encode(resp)
			conn.Close()
		}
	}()
	return &gatekeeper.Client{SocketPath: sock}
}

// fakeApp stands in for the lemongrass app on app.sock, records each request and answers with reply.
func fakeApp(t *testing.T, reply map[string]any) <-chan web.Request {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "app.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	prev := appSocketPath
	appSocketPath = func() string { return sock }
	t.Cleanup(func() { appSocketPath = prev })

	got := make(chan web.Request, 8)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			var req web.Request
			json.NewDecoder(conn).Decode(&req)
			got <- req
			json.NewEncoder(conn).Encode(reply)
			conn.Close()
		}
	}()
	return got
}

func webService(vaultClient *gatekeeper.Client) (*Service, string) {
	svc := NewService(vaultClient)
	svc.byID["abc12345"] = webTestRealID
	return svc, "abc12345"
}

func activeWeb() restergate.HTTPChannelInfo {
	return restergate.HTTPChannelInfo{Status: "active", Type: "web"}
}

func TestWebCommandReachesTheApp(t *testing.T) {
	got := fakeApp(t, map[string]any{"ok": true, "text": "page: /login", "data": []byte{0x89, 'P', 'N', 'G'}})
	svc, short := webService(fakeInfoVault(t, activeWeb(), ""))

	res, err := svc.WebCommand(short, "tab-1", "open", []string{"/login"}, map[string]string{"user": "alice"})
	if err != nil {
		t.Fatalf("WebCommand: %v", err)
	}
	if res.Text != "page: /login" || string(res.Data) != "\x89PNG" {
		t.Errorf("result = %+v, want the app's text and data", res)
	}
	req := <-got
	if req.Op != "web" || req.TabID != "tab-1" || req.ChannelID != string(webTestRealID) || req.Command != "open" ||
		len(req.Args) != 1 || req.Args[0] != "/login" || req.Flags["user"] != "alice" {
		t.Errorf("app got %+v, want op web with the tab, the real channel id, the command, args and flags", req)
	}
}

func TestWebCommandRefusals(t *testing.T) {
	tests := []struct {
		name    string
		info    restergate.HTTPChannelInfo
		tabID   string
		shortID string
		want    string
	}{
		{name: "api channel", info: restergate.HTTPChannelInfo{Status: "active", Type: "api"}, tabID: "tab-1", want: "api channel, it takes get, post"},
		{name: "old channel without a type", info: restergate.HTTPChannelInfo{Status: "active"}, tabID: "tab-1", want: "api channel"},
		{name: "expired", info: restergate.HTTPChannelInfo{Status: "expired", Type: "web"}, tabID: "tab-1", want: "this channel is expired"},
		{name: "inactive", info: restergate.HTTPChannelInfo{Status: "inactive", Type: "web"}, tabID: "tab-1", want: "this channel is inactive"},
		{name: "no tab", info: activeWeb(), want: "only work inside a lemongrass tab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fakeApp(t, map[string]any{"ok": true})
			svc, short := webService(fakeInfoVault(t, tt.info, ""))
			_, err := svc.WebCommand(short, tt.tabID, "snapshot", nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
			select {
			case req := <-got:
				t.Errorf("the app was called with %+v, want no call", req)
			default:
			}
		})
	}
}

func TestWebCommandUnknownShortID(t *testing.T) {
	svc, _ := webService(fakeInfoVault(t, activeWeb(), ""))
	if _, err := svc.WebCommand("nope0000", "tab-1", "snapshot", nil, nil); !errors.Is(err, ErrNoSuchChannel) {
		t.Errorf("err = %v, want ErrNoSuchChannel", err)
	}
}

func TestWebCommandRedactsTheRealID(t *testing.T) {
	fakeApp(t, map[string]any{"ok": false, "error": "no instance for channel " + string(webTestRealID)})
	svc, short := webService(fakeInfoVault(t, activeWeb(), ""))
	_, err := svc.WebCommand(short, "tab-1", "snapshot", nil, nil)
	if err == nil || strings.Contains(err.Error(), string(webTestRealID)) || !strings.Contains(err.Error(), "web: no instance for channel "+short) {
		t.Errorf("app error = %v, want it prefixed web: with the real id replaced by %s", err, short)
	}

	svc, short = webService(fakeInfoVault(t, restergate.HTTPChannelInfo{}, "reading "+string(webTestRealID)+".json failed"))
	_, err = svc.WebCommand(short, "tab-1", "snapshot", nil, nil)
	if err == nil || strings.Contains(err.Error(), string(webTestRealID)) || !strings.Contains(err.Error(), short) {
		t.Errorf("vault error = %v, want the real id replaced by %s", err, short)
	}
}

func TestWebCommandAppNotRunning(t *testing.T) {
	prev := appSocketPath
	appSocketPath = func() string { return filepath.Join(t.TempDir(), "app.sock") }
	t.Cleanup(func() { appSocketPath = prev })
	svc, short := webService(fakeInfoVault(t, activeWeb(), ""))
	if _, err := svc.WebCommand(short, "tab-1", "snapshot", nil, nil); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Errorf("err = %v, want the app reported unreachable", err)
	}
}

func TestWebCommandOverTheAgentSocket(t *testing.T) {
	fakeApp(t, map[string]any{"ok": true, "text": "snapshot text"})
	svc, short := webService(fakeInfoVault(t, activeWeb(), ""))
	sock := filepath.Join(t.TempDir(), "agent.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go Serve(svc, l, vault.NewFailureLimiter(2, time.Minute, time.Hour))

	client := &Client{SocketPath: sock}
	res, err := client.WebCommand(short, "tab-1", "snapshot", []string{}, map[string]string{})
	if err != nil || res.Text != "snapshot text" {
		t.Fatalf("WebCommand = %+v, %v, want the app's text", res, err)
	}

	for range 2 {
		client.WebCommand("nope0000", "tab-1", "snapshot", nil, nil)
	}
	if _, err := client.WebCommand(short, "tab-1", "snapshot", nil, nil); err == nil || strings.Contains(err.Error(), "snapshot text") {
		t.Errorf("err = %v, want the failure limiter to lock out after repeated unknown ids", err)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/restergate"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/web"
)

func TestParseWebArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    webCommand
		wantErr string
	}{
		{name: "open", args: []string{"abc", "open", "/login"}, want: webCommand{shortID: "abc", command: "open", args: []string{"/login"}, flags: map[string]string{}}},
		{name: "open as user", args: []string{"abc", "OPEN", "/x", "--user", "alice"}, want: webCommand{shortID: "abc", command: "open", args: []string{"/x"}, flags: map[string]string{"user": "alice"}}},
		{name: "open user equals", args: []string{"abc", "open", "--user=alice", "/x"}, want: webCommand{shortID: "abc", command: "open", args: []string{"/x"}, flags: map[string]string{"user": "alice"}}},
		{name: "type submit", args: []string{"abc", "type", "e3", "hello world", "--submit"}, want: webCommand{shortID: "abc", command: "type", args: []string{"e3", "hello world"}, flags: map[string]string{"submit": "true"}}},
		{name: "wait url", args: []string{"abc", "wait", "--url", "/orders/*"}, want: webCommand{shortID: "abc", command: "wait", args: []string{}, flags: map[string]string{"url": "/orders/*"}}},
		{name: "network", args: []string{"abc", "network", "--failed", "--since", "2"}, want: webCommand{shortID: "abc", command: "network", args: []string{}, flags: map[string]string{"failed": "true", "since": "2"}}},
		{name: "console errors", args: []string{"abc", "console", "--errors"}, want: webCommand{shortID: "abc", command: "console", args: []string{}, flags: map[string]string{"errors": "true"}}},
		{name: "type dashes after terminator", args: []string{"abc", "type", "--submit", "e3", "--", "--help"}, want: webCommand{shortID: "abc", command: "type", args: []string{"e3", "--help"}, flags: map[string]string{"submit": "true"}}},
		{name: "terminator then too many", args: []string{"abc", "click", "e1", "--", "--x"}, wantErr: "click takes 1 argument"},
		{name: "press", args: []string{"abc", "press", "escape"}, want: webCommand{shortID: "abc", command: "press", args: []string{"Escape"}, flags: map[string]string{}}},
		{name: "screenshot", args: []string{"abc", "screenshot", "--out", "a.png", "--confirm"}, want: webCommand{shortID: "abc", command: "screenshot", args: []string{}, flags: map[string]string{"out": "a.png", "confirm": "true"}}},
		{name: "back", args: []string{"abc", "back"}, want: webCommand{shortID: "abc", command: "back", args: []string{}, flags: map[string]string{}}},
		{name: "open without target", args: []string{"abc", "open"}, wantErr: "open takes 1 argument"},
		{name: "snapshot with argument", args: []string{"abc", "snapshot", "x"}, wantErr: "snapshot takes 0 argument"},
		{name: "snapshot within", args: []string{"abc", "snapshot", "--within", "e7"}, want: webCommand{shortID: "abc", command: "snapshot", args: []string{}, flags: map[string]string{"within": "e7"}}},
		{name: "snapshot within no ref", args: []string{"abc", "snapshot", "--within"}, wantErr: "--within needs a value"},
		{name: "user flag on click", args: []string{"abc", "click", "e1", "--user", "bob"}, wantErr: "click does not take --user"},
		{name: "type missing text", args: []string{"abc", "type", "e1"}, wantErr: "type takes 2 argument"},
		{name: "press other key", args: []string{"abc", "press", "F5"}, wantErr: "press takes Enter, Tab, Escape, Space"},
		{name: "press arrow", args: []string{"abc", "press", "arrowdown"}, want: webCommand{shortID: "abc", command: "press", args: []string{"ArrowDown"}, flags: map[string]string{}}},
		{name: "press page down", args: []string{"abc", "press", "PAGEDOWN"}, want: webCommand{shortID: "abc", command: "press", args: []string{"PageDown"}, flags: map[string]string{}}},
		{name: "press space", args: []string{"abc", "press", "Space"}, want: webCommand{shortID: "abc", command: "press", args: []string{"Space"}, flags: map[string]string{}}},
		{name: "network one request", args: []string{"abc", "network", "r12"}, want: webCommand{shortID: "abc", command: "network", args: []string{"r12"}, flags: map[string]string{}}},
		{name: "network bare id", args: []string{"abc", "network", "12"}, want: webCommand{shortID: "abc", command: "network", args: []string{"12"}, flags: map[string]string{}}},
		{name: "network quoted hash id", args: []string{"abc", "network", "#12"}, want: webCommand{shortID: "abc", command: "network", args: []string{"#12"}, flags: map[string]string{}}},
		{name: "network bad id", args: []string{"abc", "network", "e12"}, wantErr: "network takes a request id such as r12"},
		{name: "network id with flags", args: []string{"abc", "network", "r12", "--failed"}, wantErr: "network <request-id> takes no flags"},
		{name: "network two ids", args: []string{"abc", "network", "r1", "r2"}, wantErr: "network takes 0 to 1 argument"},
		{name: "wait nothing", args: []string{"abc", "wait"}, wantErr: "exactly one of --text"},
		{name: "wait two", args: []string{"abc", "wait", "--text", "ok", "--gone", "e2"}, wantErr: "exactly one of --text"},
		{name: "screenshot no out", args: []string{"abc", "screenshot"}, wantErr: "screenshot needs --out"},
		{name: "bare flag with value", args: []string{"abc", "type", "e1", "x", "--submit=yes"}, wantErr: "--submit takes no value"},
		{name: "value flag missing", args: []string{"abc", "open", "/x", "--user"}, wantErr: "--user needs a value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWebArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWebArgs: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIsWebCommandKeepsAPICommandsApart(t *testing.T) {
	for _, name := range []string{"info", "users", "flush", "get", "post", "put", "patch", "delete", "head", "options"} {
		if isWebCommand(name) {
			t.Errorf("%s is read as a web command", name)
		}
	}
}

// webStack runs a real agent over a fake vault that reports channelType, and a fake app under a temporary HOME answering with appReply, and returns an agent client and the short id.
func webStack(t *testing.T, channelType string, appReply map[string]any) (*agent.Client, string, <-chan web.Request) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)

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
			var req struct{ Op string }
			json.NewDecoder(conn).Decode(&req)
			switch req.Op {
			case "http_channel_info":
				json.NewEncoder(conn).Encode(map[string]any{"ok": true, "payload": map[string]any{"info": restergate.HTTPChannelInfo{Status: "active", Type: channelType}}})
			case "http_channel_scope":
				json.NewEncoder(conn).Encode(map[string]any{"ok": true})
			default:
				json.NewEncoder(conn).Encode(map[string]any{"ok": false, "error": "not a db channel"})
			}
			conn.Close()
		}
	}()

	os.MkdirAll(filepath.Join(dir, ".lemongrass"), 0o700)
	al, err := net.Listen("unix", web.SocketPath())
	if err != nil {
		t.Fatalf("net.Listen app: %v", err)
	}
	t.Cleanup(func() { al.Close() })
	got := make(chan web.Request, 4)
	go func() {
		for {
			conn, err := al.Accept()
			if err != nil {
				return
			}
			var req web.Request
			json.NewDecoder(conn).Decode(&req)
			got <- req
			json.NewEncoder(conn).Encode(appReply)
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

	prevTab := tabID
	tabID = "tab-1"
	t.Cleanup(func() { tabID = prevTab })
	return &agent.Client{SocketPath: agentSock}, short, got
}

func TestExecWebPrintsTheText(t *testing.T) {
	client, short, got := webStack(t, "web", map[string]any{"ok": true, "text": "url: /\n- button \"Save\" [e1]\n"})
	var out bytes.Buffer
	cmd, _ := parseWebArgs([]string{short, "snapshot"})
	if err := execWeb(client, cmd, &out); err != nil {
		t.Fatalf("execWeb: %v", err)
	}
	if out.String() != "url: /\n- button \"Save\" [e1]\n" {
		t.Errorf("printed %q, want the app's text", out.String())
	}
	if req := <-got; req.TabID != "tab-1" || req.Command != "snapshot" {
		t.Errorf("app got %+v, want the tab id and the command", req)
	}
}

func TestExecWebScreenshot(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake")
	client, short, got := webStack(t, "web", map[string]any{"ok": true, "data": png})
	dir := t.TempDir()

	var out bytes.Buffer
	cmd, _ := parseWebArgs([]string{short, "screenshot", "--out", dir})
	if err := execWeb(client, cmd, &out); err != nil {
		t.Fatalf("execWeb: %v", err)
	}
	if req := <-got; req.Flags["out"] != "" || len(req.Flags) != 0 {
		t.Errorf("app got flags %v, want --out kept on the CLI", req.Flags)
	}
	saved := filepath.Join(dir, "screenshot.png")
	if b, err := os.ReadFile(saved); err != nil || !bytes.Equal(b, png) {
		t.Fatalf("file = %q, %v, want the PNG bytes", b, err)
	}
	var printed map[string]any
	if err := json.Unmarshal(out.Bytes(), &printed); err != nil || printed["saved_to"] != saved || printed["size"] != float64(len(png)) {
		t.Errorf("printed %s, want saved_to and size", out.String())
	}

	out.Reset()
	if err := execWeb(client, cmd, &out); err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("second screenshot err = %v, want it refused naming --confirm", err)
	}
	select {
	case req := <-got:
		t.Errorf("the app was called with %+v for an existing file, want the command stopped first", req)
	default:
	}
	if b, _ := os.ReadFile(saved); !bytes.Equal(b, png) {
		t.Errorf("the existing file changed")
	}

	cmd, _ = parseWebArgs([]string{short, "screenshot", "--out", saved, "--confirm"})
	if err := execWeb(client, cmd, &out); err != nil {
		t.Errorf("screenshot with --confirm: %v", err)
	}
}

func TestExecWebOnAPIChannelNamesTheAPICommands(t *testing.T) {
	client, short, _ := webStack(t, "api", map[string]any{"ok": true})
	cmd, _ := parseWebArgs([]string{short, "snapshot"})
	if err := execWeb(client, cmd, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "get, post") {
		t.Errorf("err = %v, want the api commands named", err)
	}
}

func TestRefuseAPIOnWeb(t *testing.T) {
	client, short, _ := webStack(t, "web", map[string]any{"ok": true})
	for _, args := range [][]string{{short, "get", "/orders"}, {short, "flush"}} {
		if err := refuseAPIOnWeb(client, args); err == nil || !strings.Contains(err.Error(), "web channel, it takes open") {
			t.Errorf("%v: err = %v, want the web commands named", args, err)
		}
	}
	for _, args := range [][]string{{short, "info"}, {short, "users"}} {
		if err := refuseAPIOnWeb(client, args); err != nil {
			t.Errorf("%v: err = %v, want info and users allowed on a web channel", args, err)
		}
	}

	if err := refuseAPIOnWeb(client, []string{"nope0000", "get", "/orders"}); err == nil || !strings.Contains(err.Error(), "no such channel") {
		t.Errorf("unknown id: err = %v, want the info call's error returned so the request is not sent", err)
	}

	client, short, _ = webStack(t, "api", map[string]any{"ok": true})
	if err := refuseAPIOnWeb(client, []string{short, "get", "/orders"}); err != nil {
		t.Errorf("api channel: err = %v, want no refusal", err)
	}
}

func TestParseWebUpload(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.csv")
	b := filepath.Join(dir, "b.png")
	os.WriteFile(a, []byte("x"), 0o644)
	os.WriteFile(b, []byte("y"), 0o644)
	t.Chdir(dir)

	cmd, err := parseWebArgs([]string{"abc", "upload", "e4", "a.csv", b})
	if err != nil {
		t.Fatalf("parseWebArgs: %v", err)
	}
	if want := []string{"e4", a, b}; !reflect.DeepEqual(cmd.args, want) {
		t.Errorf("args = %v, want the ref and absolute paths %v", cmd.args, want)
	}
	for _, tc := range []struct {
		args    []string
		wantErr string
	}{
		{[]string{"abc", "upload", "e4"}, "upload takes at least 2 argument"},
		{[]string{"abc", "upload", "e4", "missing.csv"}, "does not exist"},
		{[]string{"abc", "upload", "e4", dir}, "is not a regular file"},
	} {
		if _, err := parseWebArgs(tc.args); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.wantErr)
		}
	}
}

func TestExecWebDownload(t *testing.T) {
	body := []byte("name,amount\nexample,1\n")
	client, short, got := webStack(t, "web", map[string]any{"ok": true, "text": "as alice\naction 3: downloaded", "data": body, "name": "../template.csv"})
	dir := t.TempDir()

	if _, err := parseWebArgs([]string{short, "download", "e5"}); err == nil || !strings.Contains(err.Error(), "download needs --out") {
		t.Errorf("download without --out: err = %v", err)
	}

	var out bytes.Buffer
	cmd, _ := parseWebArgs([]string{short, "download", "e5", "--out", dir})
	if err := execWeb(client, cmd, &out); err != nil {
		t.Fatalf("execWeb: %v", err)
	}
	if req := <-got; req.Command != "download" || len(req.Flags) != 0 || req.Args[0] != "e5" {
		t.Errorf("app got %+v, want download e5 with --out kept on the CLI", req)
	}
	saved := filepath.Join(dir, "template.csv")
	if b, err := os.ReadFile(saved); err != nil || !bytes.Equal(b, body) {
		t.Fatalf("file = %q, %v, want the body under the suggested name's last element", b, err)
	}
	text, last, _ := strings.Cut(strings.TrimRight(out.String(), "\n"), "\n{")
	if text != "as alice\naction 3: downloaded" {
		t.Errorf("printed %q first, want the action result", text)
	}
	var printed map[string]any
	if err := json.Unmarshal([]byte("{"+last), &printed); err != nil || printed["saved_to"] != saved || printed["size"] != float64(len(body)) || printed["name"] != "../template.csv" {
		t.Errorf("printed %s, want saved_to, size and name", out.String())
	}

	cmd, _ = parseWebArgs([]string{short, "download", "e5", "--out", saved})
	if err := execWeb(client, cmd, &out); err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("download onto an existing file: err = %v, want it refused naming --confirm", err)
	}
	select {
	case req := <-got:
		t.Errorf("the app was called with %+v for an existing file, want the command stopped before clicking", req)
	default:
	}
}

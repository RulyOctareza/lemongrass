package nodehttp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/node"
	"github.com/faizalv/lemongrass/internal/project"
)

const testBook = "garden_design[2026-01-02][soil]"

type env struct {
	srv     *httptest.Server
	token   string
	cfgPath string
	cfg     node.Config
	saves   int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	chapter := filepath.Join(dir, "biblio", "books", testBook, "beds[soil].md")
	if err := os.MkdirAll(filepath.Dir(chapter), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chapter, []byte("raised beds\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	token, hash, err := node.MintToken()
	if err != nil {
		t.Fatal(err)
	}
	e := &env{token: token, cfgPath: filepath.Join(t.TempDir(), "node.json")}
	e.cfg = node.Config{
		Projects: []node.ProjectAlias{{Alias: "garden", ProjectID: "p1"}},
		Grants: []node.Grant{{
			ID: "g1", TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour),
			Shares: []node.Share{{Alias: "garden", Books: []string{"garden_design[2026-01-02]"}}},
		}},
	}
	e.save(t)
	lib := &node.Library{Projects: func() ([]project.Project, error) {
		return []project.Project{{ID: "p1", Path: dir}}, nil
	}}
	e.srv = httptest.NewServer(NewServer(node.NewStore(e.cfgPath), lib))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) save(t *testing.T) {
	t.Helper()
	data, _ := json.Marshal(e.cfg)
	if err := os.WriteFile(e.cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	// The Store detects a rewrite by mtime and size, and two quick same-size writes can share an mtime.
	e.saves++
	later := time.Now().Add(time.Duration(e.saves) * time.Second)
	if err := os.Chtimes(e.cfgPath, later, later); err != nil {
		t.Fatal(err)
	}
}

func (e *env) do(t *testing.T, method, path, token string, body string, header ...string) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data), resp.Header
}

func TestReadRoutes(t *testing.T) {
	e := newEnv(t)
	code, body, _ := e.do(t, "GET", "/read", e.token, "")
	if code != 200 || body != "garden/books/"+testBook+"\n" {
		t.Errorf("contents: %d %q", code, body)
	}
	code, body, _ = e.do(t, "GET", "/read/garden/books/garden_design%5B2026-01-02%5D/beds%5Bsoil%5D.md", e.token, "")
	if code != 200 || body != "raised beds\n" {
		t.Errorf("chapter: %d %q", code, body)
	}
}

func TestShareEditsTakeEffectOnTheNextRequest(t *testing.T) {
	e := newEnv(t)
	const chapter = "/read/garden/books/garden_design%5B2026-01-02%5D/beds%5Bsoil%5D.md"
	if code, _, _ := e.do(t, "GET", chapter, e.token, ""); code != 200 {
		t.Fatalf("before edit: %d", code)
	}
	e.cfg.Grants[0].Shares = []node.Share{{Alias: "garden", Tasks: []string{"nothing"}}}
	e.save(t)
	if code, _, _ := e.do(t, "GET", chapter, e.token, ""); code != 404 {
		t.Errorf("after removing the book: %d, want 404", code)
	}
	e.cfg.Grants[0].Shares = []node.Share{{Alias: "garden", Books: []string{"garden_design[2026-01-02]"}}}
	e.save(t)
	if code, _, _ := e.do(t, "GET", chapter, e.token, ""); code != 200 {
		t.Errorf("after adding it back: %d, want 200", code)
	}
}

func TestUnauthorizedAndUniformNotFound(t *testing.T) {
	e := newEnv(t)
	for _, tok := range []string{"", "lgn_wrong", "nope"} {
		if code, _, _ := e.do(t, "GET", "/read", tok, ""); code != 401 {
			t.Errorf("token %q: status %d, want 401", tok, code)
		}
	}
	_, missing, _ := e.do(t, "GET", "/read/garden/books/never[2020-01-01]", e.token, "")
	for _, p := range []string{"/read/garden/laws", "/read/other", "/elsewhere", "/read/garden/books/never[2020-01-01]"} {
		code, body, _ := e.do(t, "GET", p, e.token, "")
		if code != 404 || body != missing {
			t.Errorf("%s: %d %q, want 404 %q", p, code, body, missing)
		}
	}
}

func TestWritesAreRefused(t *testing.T) {
	e := newEnv(t)
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if code, _, _ := e.do(t, m, "/read/garden", e.token, "x"); code != 405 {
			t.Errorf("%s /read: status %d, want 405", m, code)
		}
	}
	if code, _, _ := e.do(t, "GET", "/mcp", e.token, ""); code != 405 {
		t.Errorf("GET /mcp: status %d, want 405", code)
	}
}

func TestETag(t *testing.T) {
	e := newEnv(t)
	_, _, h := e.do(t, "GET", "/read", e.token, "")
	etag := h.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	if code, body, _ := e.do(t, "GET", "/read", e.token, "", "If-None-Match", etag); code != 304 || body != "" {
		t.Errorf("conditional read: %d %q, want 304", code, body)
	}
}

func TestRevokeAndExpiryTakeEffectOnTheNextRequest(t *testing.T) {
	e := newEnv(t)
	if code, _, _ := e.do(t, "GET", "/read", e.token, ""); code != 200 {
		t.Fatalf("before revoke: %d", code)
	}
	e.cfg.Grants[0].Revoked = true
	e.save(t)
	if code, _, _ := e.do(t, "GET", "/read", e.token, ""); code != 401 {
		t.Errorf("after revoke: %d, want 401", code)
	}
	e.cfg.Grants[0].Revoked = false
	e.cfg.Grants[0].ExpiresAt = time.Now().Add(-time.Second)
	e.save(t)
	if code, _, _ := e.do(t, "GET", "/read", e.token, ""); code != 401 {
		t.Errorf("after expiry: %d, want 401", code)
	}
}

func TestFailuresLockOut(t *testing.T) {
	e := newEnv(t)
	for range maxFailures {
		e.do(t, "GET", "/read", "lgn_bad", "")
	}
	if code, _, _ := e.do(t, "GET", "/read", e.token, ""); code != 429 {
		t.Errorf("after %d failures: %d, want 429", maxFailures, code)
	}
}

func rpc(t *testing.T, e *env, msg string) (int, map[string]any) {
	t.Helper()
	code, body, _ := e.do(t, "POST", "/mcp", e.token, msg, "Content-Type", "application/json", "Accept", "application/json, text/event-stream")
	var out map[string]any
	if body != "" {
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatalf("response %q: %v", body, err)
		}
	}
	return code, out
}

func TestMCPInitializeNegotiatesVersion(t *testing.T) {
	e := newEnv(t)
	_, out := rpc(t, e, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	res := out["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" {
		t.Errorf("known version not echoed: %v", res["protocolVersion"])
	}
	if res["instructions"] != instructions {
		t.Errorf("instructions missing")
	}
	_, out = rpc(t, e, `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if v := out["result"].(map[string]any)["protocolVersion"]; v != protocolVersions[0] {
		t.Errorf("unknown version answered with %v, want %v", v, protocolVersions[0])
	}
}

func TestMCPMethods(t *testing.T) {
	e := newEnv(t)
	if code, _ := rpc(t, e, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != 202 {
		t.Errorf("notification: %d, want 202", code)
	}
	if _, out := rpc(t, e, `{"jsonrpc":"2.0","id":"p","method":"ping"}`); out["id"] != "p" || out["result"] == nil {
		t.Errorf("ping: %v", out)
	}
	_, out := rpc(t, e, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	tools := out["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "read" {
		t.Errorf("tools/list: %v", tools)
	}
	_, out = rpc(t, e, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read","arguments":{"path":"garden/books/`+testBook+`/beds[soil]"}}}`)
	res := out["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"]
	if text != "raised beds\n" || res["isError"] != false {
		t.Errorf("tools/call read: %v", res)
	}
	_, out = rpc(t, e, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"read","arguments":{"path":"garden/laws"}}}`)
	if out["result"].(map[string]any)["isError"] != true {
		t.Errorf("read outside the grant not an error: %v", out)
	}
	_, out = rpc(t, e, `{"jsonrpc":"2.0","id":6,"method":"resources/list"}`)
	if out["error"].(map[string]any)["code"] != float64(codeMethodNotFound) {
		t.Errorf("unknown method: %v", out)
	}
	_, out = rpc(t, e, `not json`)
	if out["error"].(map[string]any)["code"] != float64(codeParse) {
		t.Errorf("parse error: %v", out)
	}
}

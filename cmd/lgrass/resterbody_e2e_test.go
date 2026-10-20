package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/faizalv/lemongrass/internal/vault/vaulttest"
)

func startStack(t *testing.T) (*gatekeeper.Client, *agent.Client) {
	t.Helper()
	svc, err := gatekeeper.NewBackend(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	vaultSock := filepath.Join(t.TempDir(), "vault.sock")
	vl, err := net.Listen("unix", vaultSock)
	if err != nil {
		t.Fatal(err)
	}
	go gatekeeper.Serve(svc, vl, vault.NewFailureLimiter(1000, time.Minute, time.Hour))
	t.Cleanup(func() { vl.Close() })
	vaultClient := &gatekeeper.Client{SocketPath: vaultSock}

	agentSock := filepath.Join(t.TempDir(), "agent.sock")
	al, err := net.Listen("unix", agentSock)
	if err != nil {
		t.Fatal(err)
	}
	go agent.Serve(agent.NewService(vaultClient), al, vault.NewFailureLimiter(1000, time.Minute, time.Hour))
	t.Cleanup(func() { al.Close() })
	return vaultClient, &agent.Client{SocketPath: agentSock}
}

func TestSpreadsheetUploadReachesTheAPIThroughTheWholeChain(t *testing.T) {
	want := spreadsheetBytes()
	var gotAuth string
	var gotFields, gotFile []byte
	var gotFileName, gotFileType string
	upstream := http.NewServeMux()
	upstream.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
	})
	upstream.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotFields = []byte(r.FormValue("sheet"))
		f, header, err := r.FormFile("upload")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotFile, _ = io.ReadAll(f)
		gotFileName, gotFileType = header.Filename, header.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"imported": 2})
	})
	srv := httptest.NewServer(upstream)
	defer srv.Close()

	vaultClient, agentClient := startStack(t)
	const passphrase = "correct horse"
	domain := vaulttest.Domain{
		BaseURL:         srv.URL,
		LoginEndpoint:   "/login",
		TokenPath:       "access_token",
		TokenPlacement:  vault.TokenPlacement{Kind: vault.PlacementHeader, Name: "Authorization", Prefix: "Bearer "},
		FixedTTLSeconds: 3600,
		Users:           []vault.ResterUser{{Name: "alice", Fields: map[string]string{"u": "alice"}}},
	}
	if err := vaultClient.PutRester(passphrase, "staging", domain.Rester()); err != nil {
		t.Fatal(err)
	}
	c, err := vaultClient.CreateHTTPChannel(passphrase, "ch", "staging", vault.DefaultEnvironment, vault.TypeAPI, vault.HTTPScope{Methods: []string{"POST"}}, nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	shortID, err := agentClient.RegisterChannel(c.ID)
	if err != nil {
		t.Fatal(err)
	}

	path := writeTemp(t, "Q3 report.xlsx", want)
	cmd, err := parseResterArgs([]string{shortID, "post", "/import", "--form", "sheet=Summary", "--file", "upload=" + path})
	if err != nil {
		t.Fatal(err)
	}
	body, contentType, err := buildResterBody(cmd)
	if err != nil {
		t.Fatal(err)
	}
	result, err := agentClient.RequestHTTP(cmd.shortID, cmd.user, cmd.method, cmd.path, body, contentType, cmd.headers, actorFor(cmd.shortID))
	if err != nil || result.Status != 200 {
		t.Fatalf("RequestHTTP = %+v, %v", result, err)
	}

	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want the vault to have attached the token", gotAuth)
	}
	if string(gotFields) != "Summary" || gotFileName != "Q3 report.xlsx" || gotFileType != xlsxMIME || !bytes.Equal(gotFile, want) {
		t.Errorf("upstream saw sheet %q, file %q type %q, identical bytes = %v", gotFields, gotFileName, gotFileType, bytes.Equal(gotFile, want))
	}
}

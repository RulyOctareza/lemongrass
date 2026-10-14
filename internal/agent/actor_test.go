package agent

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"github.com/faizalv/lemongrass/internal/gatekeeper"
	"github.com/faizalv/lemongrass/internal/vault"
)

func TestRequestHTTPPayloadCarriesActor(t *testing.T) {
	b, err := json.Marshal(requestHTTPPayload{ShortID: "short1", Method: "GET", Path: "/x", Actor: "tab-1"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got requestHTTPPayload
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Actor != "tab-1" {
		t.Errorf("Actor = %q, want %q", got.Actor, "tab-1")
	}
}

// TestServiceRequestHTTPForwardsActorToVault proves actor crosses the agent-to-vault IPC hop:
// the fake vault-side listener reads the raw wire payload rather than going through any real
// vault or gate, since neither consumes actor yet.
func TestServiceRequestHTTPForwardsActorToVault(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "vault.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer l.Close()

	gotActor := make(chan string, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req struct {
			Payload json.RawMessage `json:"payload"`
		}
		json.NewDecoder(conn).Decode(&req)
		var p struct {
			Actor string `json:"actor"`
		}
		json.Unmarshal(req.Payload, &p)
		gotActor <- p.Actor

		json.NewEncoder(conn).Encode(struct {
			OK      bool            `json:"ok"`
			Payload json.RawMessage `json:"payload"`
		}{OK: true, Payload: json.RawMessage(`{"result":{}}`)})
	}()

	svc := &Service{
		vaultClient: &gatekeeper.Client{SocketPath: sockPath},
		byID:        map[string]vault.ChannelID{"short1": vault.ChannelID("real1")},
	}

	if _, err := svc.RequestHTTP("short1", "alice", "GET", "/x", nil, "", nil, "tab-99"); err != nil {
		t.Fatalf("RequestHTTP: %v", err)
	}

	select {
	case actor := <-gotActor:
		if actor != "tab-99" {
			t.Errorf("vault received actor %q, want %q", actor, "tab-99")
		}
	case <-t.Context().Done():
		t.Fatal("test timed out waiting for the fake vault to receive a request")
	}
}

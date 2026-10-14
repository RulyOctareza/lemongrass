package nodesvc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/node"
	"github.com/faizalv/lemongrass/internal/project"
	"github.com/faizalv/lemongrass/internal/tunnel"
)

type loopback struct{ fail bool }

func (p loopback) Listen(ctx context.Context) (net.Listener, string, error) {
	if p.fail {
		return nil, "", errors.New("tunnel: refused")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	return ln, "http://" + ln.Addr().String(), nil
}

func setup(t *testing.T, until time.Time, provider string) (*Service, string, string) {
	t.Helper()
	token, hash, _ := node.MintToken()
	path := filepath.Join(t.TempDir(), "node.json")
	writeConfig(t, path, node.Config{
		Grants: []node.Grant{{ID: "g1", TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour)}},
		Tunnel: node.Tunnel{Provider: provider, Until: until},
	})
	lib := &node.Library{Projects: func() ([]project.Project, error) { return nil, nil }}
	svc := NewService(node.NewStore(path), lib, func(tn node.Tunnel) (tunnel.Provider, string) {
		return loopback{fail: tn.Provider == "broken"}, tn.Provider
	})
	t.Cleanup(svc.Stop)
	return svc, path, token
}

func writeConfig(t *testing.T, path string, cfg node.Config) {
	t.Helper()
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Duration(len(data)) * time.Millisecond)
	os.Chtimes(path, later, later)
}

func TestTunnelFollowsConfigAndServesWithAuth(t *testing.T) {
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	svc, path, token := setup(t, until, "fake")
	svc.Sync()
	st := svc.Status()
	if !st.Running || st.URL == "" || !st.Until.Equal(until) {
		t.Fatalf("status after open: %+v", st)
	}
	req, _ := http.NewRequest("GET", st.URL+"/read", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("read through tunnel: %d", resp.StatusCode)
	}
	if _, ok := svc.Status().LastUsed["g1"]; !ok {
		t.Errorf("last used not recorded")
	}

	writeConfig(t, path, node.Config{Tunnel: node.Tunnel{Provider: "fake", Until: time.Now().Add(-time.Minute)}})
	svc.Sync()
	if svc.Status().Running {
		t.Errorf("tunnel still open after its time ran out")
	}
	if _, err := http.Get(st.URL + "/read"); err == nil {
		t.Errorf("closed tunnel still answers")
	}
}

func TestExpiryClosesWithoutAConfigChange(t *testing.T) {
	svc, _, _ := setup(t, time.Now().Add(time.Hour), "fake")
	svc.Sync()
	svc.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	svc.Sync()
	if svc.Status().Running {
		t.Errorf("tunnel open past its until")
	}
}

func TestFailedOpenIsReportedAndNotRetriedUntilReload(t *testing.T) {
	svc, _, _ := setup(t, time.Now().Add(time.Hour), "broken")
	svc.Sync()
	st := svc.Status()
	if st.Running || st.Error == "" {
		t.Fatalf("status after a failed open: %+v", st)
	}
	if svc.failedKey == "" {
		t.Errorf("failure not remembered")
	}
}

func TestSocketOps(t *testing.T) {
	svc, _, _ := setup(t, time.Now().Add(time.Hour), "fake")
	sock := filepath.Join(t.TempDir(), "node.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go svc.Serve(l)
	for _, op := range []string{"reload", "status"} {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		json.NewEncoder(conn).Encode(request{Op: op})
		var resp response
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		if !resp.OK || resp.Status == nil || !resp.Status.Running {
			t.Errorf("%s: %+v", op, resp)
		}
	}
}

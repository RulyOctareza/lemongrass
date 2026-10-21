// Package nodesvc runs the node daemon: it keeps the tunnel matching node.json and answers the app's reload and status calls on node.sock.
package nodesvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
	"github.com/faizalv/lemongrass/internal/node"
	"github.com/faizalv/lemongrass/internal/nodehttp"
	"github.com/faizalv/lemongrass/internal/tunnel"
)

const connectTimeout = 30 * time.Second

func SocketPath() string {
	return filepath.Join(config.Dir(), "node.sock")
}

type request struct {
	Op string `json:"op"`
}

// Status is what the app shows; it holds no token. Connecting is true while the provider is
// opening the tunnel, which can take seconds.
type Status struct {
	Running    bool                 `json:"running"`
	Connecting bool                 `json:"connecting"`
	URL        string               `json:"url,omitempty"`
	Until      time.Time            `json:"until,omitzero"`
	Error      string               `json:"error,omitempty"`
	LastUsed   map[string]time.Time `json:"lastUsed"`
}

type response struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Status *Status `json:"status,omitempty"`
}

// ProviderFunc builds the tunnel provider for a config; the returned key changes whenever the tunnel must be reopened.
type ProviderFunc func(node.Tunnel) (tunnel.Provider, string)

type Service struct {
	store    *node.Store
	handler  http.Handler
	provider ProviderFunc
	now      func() time.Time

	mu        sync.Mutex
	running   *run
	failedKey string
	// heldKey is the tunnel node.json already asked for when the daemon started, which it never opens.
	heldKey    string
	connecting bool
	lastErr    string
	lastUsed   map[string]time.Time
}

type run struct {
	key    string
	until  time.Time
	url    string
	srv    *http.Server
	cancel context.CancelFunc
}

func NewService(store *node.Store, library *node.Library, provider ProviderFunc) *Service {
	s := &Service{store: store, provider: provider, now: time.Now, lastUsed: map[string]time.Time{}}
	h := nodehttp.NewServer(store, library)
	h.OnUse = s.used
	s.handler = h
	return s
}

func (s *Service) used(grantID string, at time.Time) {
	s.mu.Lock()
	s.lastUsed[grantID] = at
	s.mu.Unlock()
}

// Sync opens, reopens or closes the tunnel so it matches node.json and the clock.
func (s *Service) Sync() {
	cfg, err := s.store.Config()
	if err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.mu.Unlock()
		return
	}
	t := cfg.Tunnel
	want := t.Until.After(s.now())
	provider, key := s.tunnelKey(t)

	s.mu.Lock()
	var old *run
	if s.running != nil && (!want || s.running.key != key) {
		old, s.running = s.running, nil
	}
	if !want {
		s.failedKey, s.heldKey, s.lastErr = "", "", ""
	}
	start := want && s.running == nil && s.failedKey != key && s.heldKey != key
	s.connecting = start
	s.mu.Unlock()
	shutdown(old)
	if !start {
		return
	}

	ln, url, cancel, err := listen(provider)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connecting = false
	if err != nil {
		s.failedKey, s.lastErr = key, err.Error()
		log.Printf("nodesvc: tunnel not opened: %v", err)
		return
	}
	if s.running != nil {
		ln.Close()
		cancel()
		return
	}
	srv := nodehttp.NewHTTPServer(s.handler)
	s.running = &run{key: key, until: t.Until, url: url, srv: srv, cancel: cancel}
	s.failedKey, s.lastErr = "", ""
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("nodesvc: serving: %v", err)
		}
	}()
	log.Printf("nodesvc: tunnel open at %s until %s", url, t.Until.Format(time.RFC3339))
}

func (s *Service) tunnelKey(t node.Tunnel) (tunnel.Provider, string) {
	provider, key := s.provider(t)
	return provider, fmt.Sprintf("%s|%d", key, t.Until.UnixNano())
}

// holdLeftover keeps a tunnel that node.json still asks for at start closed, so a reboot or a
// daemon restart never reopens one; only an open written while the daemon runs changes the key.
func (s *Service) holdLeftover() {
	cfg, err := s.store.Config()
	if err != nil || !cfg.Tunnel.Until.After(s.now()) {
		return
	}
	_, key := s.tunnelKey(cfg.Tunnel)
	s.mu.Lock()
	s.heldKey = key
	s.mu.Unlock()
}

// listen gives the provider a context that lives as long as the tunnel, cancelled early only when connecting takes too long.
func listen(p tunnel.Provider) (net.Listener, string, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		ln  net.Listener
		url string
		err error
	}
	done := make(chan result, 1)
	go func() {
		ln, url, err := p.Listen(ctx)
		done <- result{ln, url, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			cancel()
			return nil, "", nil, r.err
		}
		return r.ln, r.url, cancel, nil
	case <-time.After(connectTimeout):
		cancel()
		go func() {
			if r := <-done; r.ln != nil {
				r.ln.Close()
			}
		}()
		return nil, "", nil, errors.New("nodesvc: the tunnel did not connect in time")
	}
}

// shutdown runs outside s.mu, since in-flight requests take it to record their grant's use.
func shutdown(r *run) {
	if r == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.srv.Shutdown(ctx)
	r.cancel()
	log.Printf("nodesvc: tunnel closed")
}

// Stop closes the tunnel, if open, without touching node.json.
func (s *Service) Stop() {
	s.mu.Lock()
	r := s.running
	s.running = nil
	s.mu.Unlock()
	shutdown(r)
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Error: s.lastErr, LastUsed: map[string]time.Time{}}
	for id, at := range s.lastUsed {
		st.LastUsed[id] = at
	}
	st.Connecting = s.connecting
	if s.running != nil {
		st.Running, st.URL, st.Until = true, s.running.url, s.running.until
	}
	return st
}

// Run syncs on every tick, which is what closes the tunnel when its time is up.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	s.holdLeftover()
	s.Sync()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Stop()
			return
		case <-t.C:
			s.Sync()
		}
	}
}

// Serve answers node.sock, whose ops carry no data, so a forged call can only make the daemon reread node.json or report status.
func (s *Service) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *Service) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(s.now().Add(connectTimeout + 5*time.Second))
	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(response{Error: "nodesvc: " + err.Error()})
		return
	}
	switch req.Op {
	case "reload":
		s.store.Reload()
		s.mu.Lock()
		s.failedKey = ""
		s.mu.Unlock()
		s.Sync()
		st := s.Status()
		json.NewEncoder(conn).Encode(response{OK: true, Status: &st})
	case "status":
		st := s.Status()
		json.NewEncoder(conn).Encode(response{OK: true, Status: &st})
	default:
		json.NewEncoder(conn).Encode(response{Error: "nodesvc: unknown op " + req.Op})
	}
}

// DefaultProvider maps node.json's tunnel section to ngrok or the local listener, with NGROK_AUTHTOKEN as the authtoken fallback.
func DefaultProvider(clientVersion string) ProviderFunc {
	return func(t node.Tunnel) (tunnel.Provider, string) {
		if t.Provider == "local" {
			return tunnel.Local{Port: t.LocalPort}, fmt.Sprintf("local|%d", t.LocalPort)
		}
		token := t.Authtoken
		if token == "" {
			token = os.Getenv("NGROK_AUTHTOKEN")
		}
		key := "ngrok|" + node.HashToken(token) + "|" + t.Domain
		return tunnel.Ngrok{Authtoken: token, Domain: t.Domain, ClientName: "lgrass-node", Version: clientVersion}, key
	}
}

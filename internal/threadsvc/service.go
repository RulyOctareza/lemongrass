// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package threadsvc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/faizalv/lemongrass/internal/config"
	"github.com/faizalv/lemongrass/internal/session"
)

const (
	opWake = "wake"

	dialTimeout = 300 * time.Millisecond
)

func SocketPath() string {
	return filepath.Join(config.Dir(), "threads.sock")
}

type request struct {
	Op string `json:"op"`
}

type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// A wake carries no data, so the ledger stays the only source of what is owed and a forged wake cannot inject a nudge.
type Service struct {
	store        *session.Store
	deliver      *Deliverer
	mu           sync.Mutex
	rechecking   map[string]bool
	recheckDelay time.Duration
	now          func() time.Time
}

// How long a tab that is mid-turn or showing a prompt waits before its nudge is tried again.
const defaultRecheckDelay = 10 * time.Second

func NewService(store *session.Store) *Service {
	s := &Service{
		store:        store,
		deliver:      NewDeliverer(store),
		rechecking:   map[string]bool{},
		recheckDelay: defaultRecheckDelay,
		now:          time.Now,
	}
	s.deliver.Ready = s.ready
	s.deliver.Later = s.later
	return s
}

func (s *Service) ready(tabID string) bool {
	ready, err := s.store.TabReadyForNudge(tabID, s.now())
	return err != nil || ready
}

// One pending look per tab, so a busy session is not rechecked by several timers at once.
func (s *Service) later(tabID string) {
	s.mu.Lock()
	if s.rechecking[tabID] {
		s.mu.Unlock()
		return
	}
	s.rechecking[tabID] = true
	s.mu.Unlock()
	time.AfterFunc(s.recheckDelay, func() {
		s.mu.Lock()
		delete(s.rechecking, tabID)
		s.mu.Unlock()
		s.deliver.DeliverTab(tabID)
	})
}

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
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(response{Error: "threadsvc: " + err.Error()})
		return
	}
	switch req.Op {
	case opWake:
		s.Wake()
		json.NewEncoder(conn).Encode(response{OK: true})
	default:
		json.NewEncoder(conn).Encode(response{Error: "threadsvc: unknown op " + req.Op})
	}
}

// Delivers typed nudges to supported tabs with pending rows.
func (s *Service) Wake() {
	tabs, err := s.store.PendingTabs()
	if err != nil {
		return
	}
	for _, tab := range tabs {
		s.deliver.DeliverTab(tab)
	}
}

// Scans for pending rows at start, then retries due pushes and prunes the ledger until ctx ends.
func (s *Service) Run(ctx context.Context, retryEvery, pruneEvery time.Duration) {
	s.Wake()
	retry := time.NewTicker(retryEvery)
	prune := time.NewTicker(pruneEvery)
	defer retry.Stop()
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-retry.C:
			s.retryDue()
		case <-prune.C:
			s.store.PruneNotifications(s.now())
		}
	}
}

func (s *Service) retryDue() {
	tabs, err := s.store.TabsDueForRetry(MaxAttempts, Backoff, s.now())
	if err != nil {
		return
	}
	for _, tab := range tabs {
		s.deliver.DeliverTab(tab)
	}
}

// Best effort: an error means the service is not reachable.
func Wake(socketPath string) error {
	resp, err := call(socketPath, request{Op: opWake}, 2*time.Second)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}

func call(socketPath string, req request, deadline time.Duration) (response, error) {
	conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
	if err != nil {
		return response{}, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(deadline))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return response{}, err
	}
	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return response{}, err
	}
	return resp, nil
}

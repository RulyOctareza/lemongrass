// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Package nodehttp is the node's HTTP surface: bearer auth, the plain read routes and the MCP adapter, all read-only.
package nodehttp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/node"
	"github.com/faizalv/lemongrass/internal/ratelimit"
)

const (
	maxFailures   = 30
	failureWindow = time.Minute
	lockout       = time.Minute

	maxBodyBytes = 64 * 1024
)

var errLockedOut = errors.New("nodehttp: too many failed attempts, locked out")

type Server struct {
	Store   *node.Store
	Library *node.Library
	// OnUse is told which grant made each authenticated request.
	OnUse func(grantID string, at time.Time)

	limiter *ratelimit.FailureLimiter
	now     func() time.Time
}

func NewServer(store *node.Store, library *node.Library) *Server {
	return &Server{
		Store:   store,
		Library: library,
		limiter: ratelimit.New(maxFailures, failureWindow, lockout, errLockedOut),
		now:     time.Now,
	}
}

// NewHTTPServer wraps a handler with the timeouts and header limit every node listener uses.
func NewHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 * 1024,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cfg, grant, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	switch {
	case r.URL.Path == "/mcp":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.serveMCP(w, r, cfg, grant)
	case r.URL.Path == "/read" || strings.HasPrefix(r.URL.Path, "/read/"):
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET, HEAD")
			return
		}
		s.serveRead(w, r, cfg, grant)
	default:
		http.Error(w, node.ErrNotFound.Error(), http.StatusNotFound)
	}
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (node.Config, node.Grant, bool) {
	if err := s.limiter.Check(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return node.Config{}, node.Grant{}, false
	}
	cfg, err := s.Store.Config()
	if err != nil {
		log.Printf("nodehttp: %v", err)
		http.Error(w, "nodehttp: internal error", http.StatusInternalServerError)
		return node.Config{}, node.Grant{}, false
	}
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	found = found && strings.EqualFold(scheme, "Bearer")
	now := s.now()
	grant, ok := node.Authenticate(cfg, strings.TrimSpace(token), now)
	if !found || !ok {
		s.limiter.RecordFailure()
		w.Header().Set("WWW-Authenticate", `Bearer realm="lgrass node"`)
		http.Error(w, "nodehttp: unauthorized", http.StatusUnauthorized)
		return node.Config{}, node.Grant{}, false
	}
	if s.OnUse != nil {
		s.OnUse(grant.ID, now)
	}
	return cfg, grant, true
}

func (s *Server) serveRead(w http.ResponseWriter, r *http.Request, cfg node.Config, grant node.Grant) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			http.Error(w, "nodehttp: offset must be a non-negative integer", http.StatusBadRequest)
			return
		}
		offset = n
	}
	content, err := s.Library.Read(cfg, grant, strings.TrimPrefix(r.URL.Path, "/read"), offset)
	if err != nil {
		writeReadError(w, err)
		return
	}
	body, contentType := []byte(content.Text), "text/plain; charset=utf-8"
	if content.Image != nil {
		body, contentType = content.Image, content.MIME
	}
	etag := `"` + contentHash(body) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	if r.Method == http.MethodHead {
		return
	}
	w.Write(body)
}

func writeReadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, node.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, node.ErrBadOffset):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, node.ErrNotServed):
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	case errors.Is(err, node.ErrImageTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	default:
		log.Printf("nodehttp: %v", err)
		http.Error(w, "nodehttp: internal error", http.StatusInternalServerError)
	}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	http.Error(w, "nodehttp: method not allowed, the node is read-only", http.StatusMethodNotAllowed)
}

func contentHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:16])
}

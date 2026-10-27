// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
	"github.com/go-mysql-org/go-mysql/mysql"
	mysqlserver "github.com/go-mysql-org/go-mysql/server"
)

// ServeMySQLProxy accepts and serves MySQL wire-protocol connections on ln for channel id until ln closes.
func (g *Gate) ServeMySQLProxy(ln net.Listener, id vault.ChannelID) error {
	srv := mysqlserver.NewServerWithAuth(
		"8.0.11", mysql.DEFAULT_COLLATION_ID, mysql.AUTH_CLEAR_PASSWORD, nil, nil,
		&wireProxyAuthProvider{svc: g.vault, id: id},
	)
	authHandler := wireProxyAuthHandler{}

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go serveMySQLConn(conn, srv, authHandler)
	}
}

func serveMySQLConn(conn net.Conn, srv *mysqlserver.Server, authHandler mysqlserver.AuthenticationHandler) {
	sc, err := srv.NewCustomizedConn(conn, authHandler, mysqlProxyHandler{})
	if err != nil {
		return
	}
	for {
		if err := sc.HandleCommand(); err != nil {
			return
		}
	}
}

// wireProxyAuthProvider implements server.AuthenticationProvider for AUTH_CLEAR_PASSWORD, the
// only plugin a wire-proxy listener's Server negotiates.
type wireProxyAuthProvider struct {
	svc *vault.Service
	id  vault.ChannelID
}

func (p *wireProxyAuthProvider) Validate(authPluginName string) bool {
	return authPluginName == mysql.AUTH_CLEAR_PASSWORD
}

// Authenticate checks clientAuthData against the vault's master passphrase and reactivates the channel on success.
func (p *wireProxyAuthProvider) Authenticate(_ *mysqlserver.Conn, authPluginName string, clientAuthData []byte) error {
	if authPluginName != mysql.AUTH_CLEAR_PASSWORD {
		return mysqlserver.ErrAccessDenied
	}
	password := strings.TrimSuffix(string(clientAuthData), "\x00")

	if err := p.svc.VerifyPassphrase(password); err != nil {
		return mysqlserver.ErrAccessDenied
	}

	c, err := p.svc.ChannelScope(p.id)
	if err != nil {
		return mysqlserver.ErrAccessDenied
	}
	if c.Expired(time.Now()) {
		return mysqlserver.ErrAccessDenied
	}

	// Re-derives and caches the channel key like Activate, without extending ExpiresAt.
	if _, err := p.svc.Activate(password, p.id, c.ExpiresAt.Sub(time.Now())); err != nil {
		return mysqlserver.ErrAccessDenied
	}
	return nil
}

// wireProxyAuthHandler implements server.AuthenticationHandler for a wire-proxy listener; GetCredential returns an unused placeholder credential.
type wireProxyAuthHandler struct{}

func (wireProxyAuthHandler) GetCredential(_ string) (mysqlserver.Credential, bool, error) {
	return mysqlserver.Credential{Passwords: []string{""}, AuthPluginName: mysql.AUTH_CLEAR_PASSWORD}, true, nil
}

func (wireProxyAuthHandler) OnAuthSuccess(_ *mysqlserver.Conn) error { return nil }

func (wireProxyAuthHandler) OnAuthFailure(_ *mysqlserver.Conn, _ error) {}

// errQueryPathNotBuilt is mysqlProxyHandler's answer to every command until query execution is wired up.
var errQueryPathNotBuilt = errors.New("dbgate: wire proxy query path not built yet")

// mysqlProxyHandler is the interim server.Handler for wire-proxy connections.
type mysqlProxyHandler struct{}

func (mysqlProxyHandler) UseDB(_ string) error { return nil }

func (mysqlProxyHandler) HandleQuery(_ string) (*mysql.Result, error) {
	return nil, errQueryPathNotBuilt
}

func (mysqlProxyHandler) HandleFieldList(_, _ string) ([]*mysql.Field, error) {
	return nil, errQueryPathNotBuilt
}

func (mysqlProxyHandler) HandleStmtPrepare(_ string) (int, int, any, error) {
	return 0, 0, nil, errQueryPathNotBuilt
}

func (mysqlProxyHandler) HandleStmtExecute(_ any, _ string, _ []any) (*mysql.Result, error) {
	return nil, errQueryPathNotBuilt
}

func (mysqlProxyHandler) HandleStmtClose(_ any) error { return nil }

func (mysqlProxyHandler) HandleOtherCommand(_ byte, _ []byte) error {
	return errQueryPathNotBuilt
}

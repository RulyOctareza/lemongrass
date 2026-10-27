// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package dbgate runs database work for a channel: it classifies a statement, enforces the channel's scope, pools connections and executes. Credentials come from the vault and never persist here.
package dbgate

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/faizalv/lemongrass/internal/channelaudit"
	"github.com/faizalv/lemongrass/internal/vault"
)

type Gate struct {
	vault *vault.Service

	// Audit is the shared connector audit log. Nil in a Gate built for a test that doesn't care about it; always set on the Gate a running vault daemon serves.
	Audit *channelaudit.Store

	mu    sync.Mutex
	conns map[vault.ChannelID]*sql.DB
}

// New returns a Gate reading credentials from svc and dropping a channel's pooled connection whenever the vault invalidates that channel.
func New(svc *vault.Service) *Gate {
	g := &Gate{vault: svc, conns: make(map[vault.ChannelID]*sql.DB)}
	svc.OnInvalidate(g.closeDB)
	return g
}

// TestConnection opens connString directly and pings it, with no root secret involved since the string never touches the credential store.
func (g *Gate) TestConnection(connString string) error {
	db, err := openDB(connString)
	if err != nil {
		return err
	}
	defer db.Close()
	return pingWithTimeout(db)
}

// TestConnectionSaved decrypts dbName's stored credential and pings it, to check an
// already-saved connection still works without creating a channel against it.
func (g *Gate) TestConnectionSaved(rootSecret, dbName string) error {
	return g.withConnection(rootSecret, dbName, func(db *sql.DB, _ Engine) error {
		return pingWithTimeout(db)
	})
}

// ConnectionSSHHosts returns the ssh host alias of every saved connection that names one, keyed
// by connection name, for the Connections list.
func (g *Gate) ConnectionSSHHosts(rootSecret string) (map[string]string, error) {
	hosts := make(map[string]string)
	err := g.vault.WithConnections(rootSecret, func(dbName string, connString []byte) error {
		// A stored alias that fails validation is left out here; Test reports it on that connection.
		if _, alias, err := splitSSHAlias(string(connString)); err == nil && alias != "" {
			hosts[dbName] = alias
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

// ListTables decrypts dbName's stored credential and lists the tables in its database, for
// the Channels form's table picker.
func (g *Gate) ListTables(rootSecret, dbName string) ([]string, error) {
	var tables []string
	err := g.withConnection(rootSecret, dbName, func(db *sql.DB, engine Engine) error {
		found, err := listTables(db, engine)
		if err != nil {
			return err
		}
		tables = found
		return nil
	})
	return tables, err
}

// UpdateConnection replaces the stored connection string dbName with connString and re-wraps it
// for every channel minted from it. The engine cannot change because a channel's scope was built
// against it.
func (g *Gate) UpdateConnection(rootSecret, dbName, connString string) error {
	return g.vault.ReplaceConnection(rootSecret, dbName, connString, func(old string) error {
		oldEngine, err := engineOf(old)
		if err != nil {
			return err
		}
		newEngine, err := engineOf(connString)
		if err != nil {
			return err
		}
		if oldEngine != newEngine {
			return errors.New("dbgate: connection " + dbName + " is a " + string(oldEngine) + " connection, the engine cannot change")
		}
		return nil
	})
}

// withConnection decrypts dbName's credential, opens a connection with it, passes both to fn, and always closes the connection before returning.
func (g *Gate) withConnection(rootSecret, dbName string, fn func(*sql.DB, Engine) error) error {
	return g.vault.WithConnection(rootSecret, dbName, func(connString []byte) error {
		engine, err := engineOf(string(connString))
		if err != nil {
			return err
		}
		db, err := openDB(string(connString))
		if err != nil {
			return err
		}
		defer db.Close()
		return fn(db, engine)
	})
}

// Query classifies sqlText and checks it against the channel's scope, since scope is the real security boundary. It rejects a write-kind statement outright, mirroring Execute's own rejection of a read-kind one, so a caller can't commit through the read path or silently no-op a write through it.
func (g *Gate) Query(id vault.ChannelID, sqlText string) (QueryResult, error) {
	c, connBytes, err := g.vault.OpenChannel(id)
	if err != nil {
		return QueryResult{}, err
	}
	defer vault.Zero(connBytes)
	connString := string(connBytes)

	engine, err := engineOf(connString)
	if err != nil {
		return QueryResult{}, err
	}
	stmt, err := ClassifyQuery(engine, sqlText)
	if err != nil {
		return QueryResult{}, err
	}
	if isWriteKind(stmt.Kind) {
		return QueryResult{}, errors.New("dbgate: use Execute for a write statement")
	}
	if err := AllowStatement(c.Scope, stmt); err != nil {
		return QueryResult{}, err
	}

	db, err := g.getDB(id, connString)
	if err != nil {
		return QueryResult{}, errors.New("dbgate: the stored connection could not be opened")
	}
	result, err := runQuery(db, stmt, sqlText)
	if err != nil {
		return QueryResult{}, err
	}
	if stmt.ListsTables {
		result = FilterTableList(c.Scope, result)
	}
	return result, nil
}

// isWriteKind reports whether k is one Execute handles rather than Query.
func isWriteKind(k Kind) bool {
	return k == KindInsert || k == KindUpdate || k == KindDelete
}

// WriteResult is a write statement's shaped output: no rows, since a write has none to show.
type WriteResult struct {
	AffectedRows int64 `json:"affected_rows"`
	DryRun       bool  `json:"dry_run"`
}

// Execute classifies sqlText and checks it against the channel's scope, same order Query uses, then runs it inside a transaction: committed when commit is true, rolled back otherwise so nothing persists from a dry run. It rejects a read-kind statement outright, the mirror of Query's own rejection above. actor is the caller-facing identity the resulting audit row is stamped under, never verified and never a security boundary.
func (g *Gate) Execute(id vault.ChannelID, sqlText string, commit bool, actor string) (WriteResult, error) {
	c, connBytes, err := g.vault.OpenChannel(id)
	if err != nil {
		return WriteResult{}, err
	}
	defer vault.Zero(connBytes)
	connString := string(connBytes)

	engine, err := engineOf(connString)
	if err != nil {
		return WriteResult{}, err
	}
	stmt, err := ClassifyQuery(engine, sqlText)
	if err != nil {
		return WriteResult{}, err
	}
	if !isWriteKind(stmt.Kind) {
		return WriteResult{}, errors.New("dbgate: use Query for a read statement")
	}
	if err := AllowStatement(c.Scope, stmt); err != nil {
		return WriteResult{}, err
	}

	db, err := g.getDB(id, connString)
	if err != nil {
		return WriteResult{}, errors.New("dbgate: the stored connection could not be opened")
	}
	return g.runWrite(id, actor, stmt, db, sqlText, commit)
}

// runWrite runs sqlText inside a transaction and resolves it, committing when commit is true and rolling back otherwise, or rolling back regardless if the statement itself failed. The audit row is written right after resolution either way, so a rejection earlier in Execute (classify, scope) that never opened a transaction never produces one, but an attempt that reached the database and failed there still does.
func (g *Gate) runWrite(id vault.ChannelID, actor string, stmt Statement, db *sql.DB, sqlText string, commit bool) (WriteResult, error) {
	tx, err := db.Begin()
	if err != nil {
		return WriteResult{}, fmt.Errorf("dbgate: beginning transaction: %s", describeDBError(err))
	}
	// A refusal here writes no audit row, since the statement itself never ran.
	if err := prepareTx(tx, stmt); err != nil {
		tx.Rollback()
		return WriteResult{}, err
	}

	res, execErr := tx.Exec(sqlText)
	var affected int64
	if execErr == nil {
		affected, execErr = res.RowsAffected()
	}

	var resolveErr error
	if execErr != nil || !commit {
		resolveErr = tx.Rollback()
	} else {
		resolveErr = tx.Commit()
	}
	g.auditWrite(id, actor, stmt, affected, commit, execErr)

	if execErr != nil {
		return WriteResult{}, fmt.Errorf("dbgate: executing statement: %s", describeDBError(execErr))
	}
	if resolveErr != nil {
		verb := "committing"
		if !commit {
			verb = "rolling back"
		}
		return WriteResult{}, fmt.Errorf("dbgate: %s transaction: %s", verb, describeDBError(resolveErr))
	}
	return WriteResult{AffectedRows: affected, DryRun: !commit}, nil
}

// auditWrite records a write call's outcome, best-effort: nil Audit (a Gate built for a test that doesn't set it) and an insert failure are both silently skipped, since an audit-store outage must never block a write the transaction itself already resolved.
func (g *Gate) auditWrite(id vault.ChannelID, actor string, stmt Statement, affected int64, commit bool, execErr error) {
	if g.Audit == nil {
		return
	}
	verb := "dry_run"
	if commit {
		verb = "commit"
	}
	status := fmt.Sprintf("%s affected=%d", verb, affected)
	if execErr != nil {
		status = "error"
	}
	_ = g.Audit.Insert(channelaudit.Row{
		ChannelID:   string(id),
		ChannelKind: "db",
		Actor:       actor,
		Action:      string(stmt.Kind),
		Target:      strings.Join(stmt.WriteTables, ","),
		Status:      status,
	})
}

// getDB returns id's cached *sql.DB, opening and caching one on first use.
func (g *Gate) getDB(id vault.ChannelID, connString string) (*sql.DB, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if db, ok := g.conns[id]; ok {
		return db, nil
	}
	db, err := openDB(connString)
	if err != nil {
		return nil, err
	}
	g.conns[id] = db
	return db, nil
}

// closeDB closes and evicts id's cached *sql.DB if any, best-effort since a close error here doesn't change the caller's own outcome.
func (g *Gate) closeDB(id vault.ChannelID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if db, ok := g.conns[id]; ok {
		db.Close()
		delete(g.conns, id)
	}
}

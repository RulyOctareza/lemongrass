// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"weak"

	"github.com/jackc/pgx/v5/pgconn"
)

// statementLimit is how long the caller's statement may run before the server stops it. It is a var only so the real-database proof can lower it.
var statementLimit = 30 * time.Second

// statementBackstop is how much longer than statementLimit the client waits before closing the connection itself, for when the server-side stop never lands.
const statementBackstop = 10 * time.Second

// killDialTimeout bounds the one-off connection that sends KILL QUERY.
const killDialTimeout = 5 * time.Second

func errStatementLimit() error {
	return fmt.Errorf("dbgate: the statement ran past the %s limit and was stopped", statementLimit)
}

// statementContext bounds only the caller's statement, never BEGIN or COMMIT, so the backstop can't leave a commit's outcome unknown.
func statementContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), statementLimit+statementBackstop)
}

// postgresStatementTimeout is the limit as statement_timeout's millisecond value.
func postgresStatementTimeout() string {
	return strconv.FormatInt(statementLimit.Milliseconds(), 10)
}

// isPostgresStatementTimeout tells the server's own statement_timeout stop apart from a cancel sent by someone else, which shares its code.
func isPostgresStatementTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "57014" && strings.Contains(pgErr.Message, "statement timeout")
}

// killConnectors maps each MySQL *sql.DB openDB built to its connector, so the killer can dial outside the channel's pool, which may have no free slot. The weak key lets the entry go when the DB itself is collected.
var killConnectors sync.Map

func registerKillConnector(db *sql.DB, c driver.Connector) {
	key := weak.Make(db)
	killConnectors.Store(key, c)
	runtime.AddCleanup(db, func(k weak.Pointer[sql.DB]) { killConnectors.Delete(k) }, key)
}

func killConnectorFor(db *sql.DB) driver.Connector {
	c, ok := killConnectors.Load(weak.Make(db))
	if !ok {
		return nil
	}
	return c.(driver.Connector)
}

// mysqlKiller stops a MySQL statement at the limit with KILL QUERY from a second connection, since closing the client side leaves the query running on the server and max_execution_time covers only SELECT. The mutex makes stop and the kill exclusive, so once stop returns no KILL can still land on the connection.
type mysqlKiller struct {
	mu      sync.Mutex
	timer   *time.Timer
	stopped bool
	fired   bool
}

// armMySQLKiller starts the limit for the statement running on connection id. With no registered connector there is no way to reach the server, and only the client-side backstop applies.
func armMySQLKiller(db *sql.DB, id uint64) *mysqlKiller {
	k := &mysqlKiller{}
	connector := killConnectorFor(db)
	k.timer = time.AfterFunc(statementLimit, func() {
		k.mu.Lock()
		defer k.mu.Unlock()
		if k.stopped {
			return
		}
		k.fired = true
		if connector != nil {
			killMySQLQuery(connector, id)
		}
	})
	return k
}

// stop ends the limit, waiting out a KILL in flight, and reports whether the limit was reached.
func (k *mysqlKiller) stop() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.stopped = true
	k.timer.Stop()
	return k.fired
}

// killMySQLQuery is best-effort: if it fails, the backstop deadline still ends the call.
func killMySQLQuery(connector driver.Connector, id uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), killDialTimeout)
	defer cancel()
	db := sql.OpenDB(connector)
	defer db.Close()
	_, _ = db.ExecContext(ctx, "KILL QUERY "+strconv.FormatUint(id, 10))
}

// mysqlConnectionIDQuery reads the server's id for the current connection.
const mysqlConnectionIDQuery = "SELECT CONNECTION_ID()"

// mysqlConnectionID reads the server's id for the connection tx runs on, the id KILL QUERY needs.
func mysqlConnectionID(tx *sql.Tx) (uint64, error) {
	var id uint64
	if err := tx.QueryRow(mysqlConnectionIDQuery).Scan(&id); err != nil {
		return 0, fmt.Errorf("dbgate: preparing transaction: %s", describeDBError(err))
	}
	return id, nil
}

// statementStop holds how the caller's statement is stopped at the limit on its engine.
type statementStop struct {
	db     *sql.DB
	engine Engine
	connID uint64
	killer *mysqlKiller
}

// arm starts the limit right before the caller's statement.
func (s *statementStop) arm() {
	if s.engine == EngineMySQL {
		s.killer = armMySQLKiller(s.db, s.connID)
	}
}

// disarm ends the limit right after the statement and its rows, before COMMIT or ROLLBACK, and reports whether the limit was reached.
func (s *statementStop) disarm() bool {
	if s.killer == nil {
		return false
	}
	return s.killer.stop()
}

// statementError reports a failed statement step, as the limit error when the server's own statement_timeout stopped it.
func statementError(step string, err error) error {
	if isPostgresStatementTimeout(err) {
		return errStatementLimit()
	}
	return fmt.Errorf("dbgate: %s: %s", step, describeDBError(err))
}

// discardConn closes conn's driver connection instead of returning it to the pool, so a KILL QUERY that landed after the statement ended can't reach the next caller.
func discardConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	conn.Close()
}

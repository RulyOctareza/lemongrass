// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

// limitServer is a fake server whose connections record what they were asked, in order, and whose "SLOW" statements run until a KILL QUERY names their connection.
type limitServer struct {
	mu      sync.Mutex
	events  []string
	nextID  uint64
	killed  map[uint64]chan struct{}
	guards  [][]driver.NamedValue
	pgError *pgconn.PgError
}

func newLimitServer() *limitServer {
	return &limitServer{killed: make(map[uint64]chan struct{})}
}

func (s *limitServer) record(event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *limitServer) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.events...)
}

// index returns the position of the first event starting with prefix, or -1.
func (s *limitServer) index(prefix string) int {
	for i, e := range s.snapshot() {
		if strings.HasPrefix(e, prefix) {
			return i
		}
	}
	return -1
}

func (s *limitServer) Connect(context.Context) (driver.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := s.nextID
	s.killed[id] = make(chan struct{})
	s.events = append(s.events, "connect "+strconv.FormatUint(id, 10))
	return &limitConn{srv: s, id: id}, nil
}

func (s *limitServer) Driver() driver.Driver { return nil }

type limitConn struct {
	srv *limitServer
	id  uint64
}

func (c *limitConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }

func (c *limitConn) Close() error {
	c.srv.record("close " + strconv.FormatUint(c.id, 10))
	return nil
}

func (c *limitConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *limitConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.srv.record("begin")
	return limitTx{c}, nil
}

// run plays the caller's statement: a SLOW one waits for its KILL or its context.
func (c *limitConn) run(ctx context.Context, query string) error {
	if !strings.Contains(query, "SLOW") {
		c.srv.record("user")
		return nil
	}
	c.srv.record("user start")
	if c.srv.pgError != nil {
		return c.srv.pgError
	}
	c.srv.mu.Lock()
	killed := c.srv.killed[c.id]
	c.srv.mu.Unlock()
	select {
	case <-killed:
		c.srv.record("user interrupted")
		return &mysql.MySQLError{Number: 1317, Message: "Query execution was interrupted"}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *limitConn) QueryContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, postgresLookupMarker) {
		c.srv.record("lookup")
		return &fakeRows{cols: []string{"kind", "name"}}, nil
	}
	if query == mysqlConnectionIDQuery {
		c.srv.record("connid")
		return &fakeRows{cols: []string{"id"}, rows: [][]driver.Value{{int64(c.id)}}}, nil
	}
	if err := c.run(ctx, query); err != nil {
		return nil, err
	}
	return &fakeRows{cols: []string{"n"}, rows: [][]driver.Value{{int64(1)}}}, nil
}

func (c *limitConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == postgresTxGuard {
		c.srv.mu.Lock()
		c.srv.guards = append(c.srv.guards, args)
		c.srv.mu.Unlock()
		c.srv.record("pins")
		return driver.RowsAffected(0), nil
	}
	if target, ok := strings.CutPrefix(query, "KILL QUERY "); ok {
		id, _ := strconv.ParseUint(target, 10, 64)
		c.srv.record("kill " + target)
		c.srv.mu.Lock()
		if ch, ok := c.srv.killed[id]; ok {
			close(ch)
			delete(c.srv.killed, id)
		}
		c.srv.mu.Unlock()
		return driver.RowsAffected(0), nil
	}
	if err := c.run(ctx, query); err != nil {
		return nil, err
	}
	return driver.RowsAffected(1), nil
}

type limitTx struct{ c *limitConn }

func (t limitTx) Commit() error   { t.c.srv.record("commit"); return nil }
func (t limitTx) Rollback() error { t.c.srv.record("rollback"); return nil }

func withStatementLimit(t *testing.T, d time.Duration) {
	t.Helper()
	old := statementLimit
	statementLimit = d
	t.Cleanup(func() { statementLimit = old })
}

func openLimitDB(t *testing.T, srv *limitServer, killable bool) *sql.DB {
	t.Helper()
	db := sql.OpenDB(srv)
	if killable {
		registerKillConnector(db, srv)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func isLimitError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "dbgate: the statement ran past the")
}

func TestRunQueryMySQLStopsAtTheLimit(t *testing.T) {
	withStatementLimit(t, 50*time.Millisecond)
	srv := newLimitServer()
	db := openLimitDB(t, srv, true)

	sqlText := "SELECT id FROM employees /* SLOW */"
	_, err := runQuery(db, classifyFor(t, EngineMySQL, "SELECT id FROM employees"), sqlText)
	if !isLimitError(err) {
		t.Fatalf("runQuery = %v, want the limit error", err)
	}
	connid, start, kill, rollback := srv.index("connid"), srv.index("user start"), srv.index("kill 1"), srv.index("rollback")
	if !(connid < start && start < kill && kill < rollback) {
		t.Errorf("events = %v, want connid, statement, kill 1, rollback in order", srv.snapshot())
	}
	if srv.index("close 1") < 0 {
		t.Errorf("events = %v, want the killed connection 1 discarded", srv.snapshot())
	}

	// The next call must run on a fresh connection, not the one a KILL was aimed at.
	if _, err := runQuery(db, classifyFor(t, EngineMySQL, "SELECT id FROM employees"), "SELECT id FROM employees"); err != nil {
		t.Fatalf("runQuery after the limit: %v", err)
	}
	if srv.index("connect 3") < 0 {
		t.Errorf("events = %v, want a fresh connection 3 after the killer's connection 2", srv.snapshot())
	}
}

func TestRunWriteMySQLStopsAtTheLimitWithoutCommitting(t *testing.T) {
	withStatementLimit(t, 50*time.Millisecond)
	srv := newLimitServer()
	db := openLimitDB(t, srv, true)

	sqlText := "UPDATE employees SET name = 'a' WHERE id = 1 /* SLOW */"
	g := &Gate{}
	_, err := g.runWrite("ch", "tester", classifyFor(t, EngineMySQL, "UPDATE employees SET name = 'a' WHERE id = 1"), db, sqlText, true)
	if !isLimitError(err) {
		t.Fatalf("runWrite = %v, want the limit error", err)
	}
	if srv.index("commit") >= 0 {
		t.Errorf("events = %v, a stopped write must not commit", srv.snapshot())
	}
	if kill, rollback := srv.index("kill 1"), srv.index("rollback"); kill < 0 || kill > rollback {
		t.Errorf("events = %v, want kill 1 before rollback", srv.snapshot())
	}
}

func TestRunWriteMySQLDisarmsBeforeCommit(t *testing.T) {
	withStatementLimit(t, 50*time.Millisecond)
	srv := newLimitServer()
	db := openLimitDB(t, srv, true)

	sqlText := "UPDATE employees SET name = 'a' WHERE id = 1"
	g := &Gate{}
	if _, err := g.runWrite("ch", "tester", classifyFor(t, EngineMySQL, sqlText), db, sqlText, true); err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	time.Sleep(3 * statementLimit)
	if srv.index("kill") >= 0 {
		t.Errorf("events = %v, the limit must be disarmed before COMMIT", srv.snapshot())
	}
	if srv.index("commit") < 0 || srv.index("close") >= 0 {
		t.Errorf("events = %v, want a commit and the connection kept in the pool", srv.snapshot())
	}
}

func TestRunQueryPostgresSetsTheLimitAtTheServer(t *testing.T) {
	withStatementLimit(t, 1500*time.Millisecond)
	srv := newLimitServer()
	db := openLimitDB(t, srv, false)

	if _, err := runQuery(db, classifyFor(t, EnginePostgres, "SELECT id FROM employees"), "SELECT id FROM employees"); err != nil {
		t.Fatalf("runQuery: %v", err)
	}
	if len(srv.guards) != 1 || len(srv.guards[0]) != 1 || srv.guards[0][0].Value != "1500" {
		t.Errorf("guard args = %v, want statement_timeout 1500", srv.guards)
	}
	if !strings.Contains(postgresTxGuard, "set_config('statement_timeout', $1, true)") {
		t.Errorf("postgresTxGuard does not set statement_timeout transaction-local: %s", postgresTxGuard)
	}
}

func TestPostgresStatementTimeoutMapsToTheLimit(t *testing.T) {
	cases := []struct {
		message string
		limit   bool
	}{
		{"canceling statement due to statement timeout", true},
		{"canceling statement due to user request", false},
	}
	for _, c := range cases {
		srv := newLimitServer()
		srv.pgError = &pgconn.PgError{Code: "57014", Message: c.message}
		db := openLimitDB(t, srv, false)

		_, readErr := runQuery(db, classifyFor(t, EnginePostgres, "SELECT id FROM employees"), "SELECT id FROM employees /* SLOW */")
		g := &Gate{}
		_, writeErr := g.runWrite("ch", "tester", classifyFor(t, EnginePostgres, "UPDATE employees SET name = 'a' WHERE id = 1"), db, "UPDATE employees SET name = 'a' WHERE id = 1 /* SLOW */", true)
		for name, err := range map[string]error{"read": readErr, "write": writeErr} {
			if isLimitError(err) != c.limit {
				t.Errorf("%s with %q: error = %v, want limit error %v", name, c.message, err, c.limit)
			}
		}
	}
}

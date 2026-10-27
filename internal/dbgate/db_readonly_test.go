// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

// txLog records what a fakeConnector's connections were asked to do, in order, and scripts the answers to the catalog lookups.
type txLog struct {
	readOnly  []bool
	stmts     []string
	commits   int
	rollbacks int
	// failOn makes any statement containing it fail.
	failOn string
	// shadowed is the name the catalog lookup reports as a user function, "" for none.
	shadowed string
	version  string
}

// Markers that tell the vault's own statements apart from the caller's in txLog.stmts.
const (
	mysqlLookupMarker    = "information_schema.ROUTINES"
	postgresLookupMarker = "pg_catalog.pg_proc"
)

type fakeConnector struct{ log *txLog }

func (c fakeConnector) Connect(context.Context) (driver.Conn, error) {
	return &fakeConn{log: c.log}, nil
}
func (c fakeConnector) Driver() driver.Driver { return nil }

type fakeConn struct{ log *txLog }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *fakeConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.log.readOnly = append(c.log.readOnly, opts.ReadOnly)
	return fakeTx{log: c.log}, nil
}

func (c *fakeConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.log.stmts = append(c.log.stmts, query)
	if c.log.failOn != "" && strings.Contains(query, c.log.failOn) {
		return nil, errors.New("refused by the fake database")
	}
	switch {
	case strings.Contains(query, mysqlLookupMarker):
		return c.lookupRows([]string{"name"}, []driver.Value{c.log.shadowed}), nil
	case strings.Contains(query, postgresLookupMarker):
		return c.lookupRows([]string{"kind", "name"}, []driver.Value{"function", c.log.shadowed}), nil
	case query == "SELECT VERSION()":
		return &fakeRows{cols: []string{"version"}, rows: [][]driver.Value{{c.log.version}}}, nil
	}
	return &fakeRows{cols: []string{"n"}, rows: [][]driver.Value{{int64(1)}}}, nil
}

func (c *fakeConn) lookupRows(cols []string, row []driver.Value) driver.Rows {
	if c.log.shadowed == "" {
		return &fakeRows{cols: cols}
	}
	return &fakeRows{cols: cols, rows: [][]driver.Value{row}}
}

func (c *fakeConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.log.stmts = append(c.log.stmts, query)
	if c.log.failOn != "" && strings.Contains(query, c.log.failOn) {
		return nil, errors.New("refused by the fake database")
	}
	return driver.RowsAffected(1), nil
}

type fakeTx struct{ log *txLog }

func (t fakeTx) Commit() error   { t.log.commits++; return nil }
func (t fakeTx) Rollback() error { t.log.rollbacks++; return nil }

type fakeRows struct {
	cols []string
	rows [][]driver.Value
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(dest, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

func openFakeDB(t *testing.T, log *txLog) *sql.DB {
	t.Helper()
	db := sql.OpenDB(fakeConnector{log: log})
	t.Cleanup(func() { db.Close() })
	return db
}

// stmtKinds labels each recorded statement, so a test can assert the order without repeating the vault's SQL.
func stmtKinds(stmts []string) []string {
	out := make([]string, len(stmts))
	for i, s := range stmts {
		switch {
		case s == postgresTxGuard:
			out[i] = "pins"
		case strings.Contains(s, mysqlLookupMarker), strings.Contains(s, postgresLookupMarker):
			out[i] = "lookup"
		case s == "SELECT VERSION()":
			out[i] = "version"
		default:
			out[i] = "user"
		}
	}
	return out
}

func classifyFor(t *testing.T, engine Engine, sqlText string) Statement {
	t.Helper()
	stmt, err := ClassifyQuery(engine, sqlText)
	if err != nil {
		t.Fatalf("ClassifyQuery(%q): %v", sqlText, err)
	}
	return stmt
}

func TestRunQueryOrderInReadOnlyTransaction(t *testing.T) {
	cases := []struct {
		engine Engine
		sql    string
		want   []string
	}{
		{EngineMySQL, "SELECT id FROM employees", []string{"user"}},
		{EngineMySQL, "SELECT UPPER(name) FROM employees", []string{"lookup", "user"}},
		{EnginePostgres, "SELECT id FROM employees", []string{"pins", "user"}},
		{EnginePostgres, "SELECT upper(name) FROM employees", []string{"pins", "lookup", "user"}},
	}
	for _, c := range cases {
		t.Run(string(c.engine)+" "+c.sql, func(t *testing.T) {
			log := &txLog{}
			result, err := runQuery(openFakeDB(t, log), classifyFor(t, c.engine, c.sql), c.sql)
			if err != nil {
				t.Fatalf("runQuery: %v", err)
			}
			if len(result.Rows) != 1 {
				t.Errorf("rows = %v, want one", result.Rows)
			}
			if !reflect.DeepEqual(log.readOnly, []bool{true}) {
				t.Errorf("BeginTx ReadOnly = %v, want [true]", log.readOnly)
			}
			if got := stmtKinds(log.stmts); !reflect.DeepEqual(got, c.want) {
				t.Errorf("statements = %v, want %v", got, c.want)
			}
			if log.rollbacks != 1 || log.commits != 0 {
				t.Errorf("rollbacks = %d, commits = %d, want 1 and 0", log.rollbacks, log.commits)
			}
		})
	}
}

func TestRunQueryRefusesShadowedFunction(t *testing.T) {
	for _, engine := range []Engine{EngineMySQL, EnginePostgres} {
		t.Run(string(engine), func(t *testing.T) {
			log := &txLog{shadowed: "upper"}
			sqlText := "SELECT upper(name) FROM employees"
			_, err := runQuery(openFakeDB(t, log), classifyFor(t, engine, sqlText), sqlText)
			var notAllowed *ErrFunctionNotAllowed
			if !errors.As(err, &notAllowed) || notAllowed.Name != "upper" {
				t.Fatalf("runQuery = %v, want function upper refused", err)
			}
			for _, kind := range stmtKinds(log.stmts) {
				if kind == "user" {
					t.Error("the caller's SQL ran after the catalog check refused it")
				}
			}
			if log.rollbacks != 1 {
				t.Errorf("rollbacks = %d, want 1", log.rollbacks)
			}
		})
	}
}

func TestRunQueryFailsClosedWhenTheLookupFails(t *testing.T) {
	cases := []struct {
		engine  Engine
		failOn  string
		version string
	}{
		{EngineMySQL, mysqlLookupMarker, "8.4.3"},
		{EnginePostgres, postgresLookupMarker, ""},
		{EnginePostgres, "set_config", ""},
	}
	for _, c := range cases {
		t.Run(string(c.engine)+" "+c.failOn, func(t *testing.T) {
			log := &txLog{failOn: c.failOn, version: c.version}
			sqlText := "SELECT upper(name) FROM employees"
			if _, err := runQuery(openFakeDB(t, log), classifyFor(t, c.engine, sqlText), sqlText); err == nil {
				t.Fatal("runQuery: expected an error, got nil")
			}
			for _, kind := range stmtKinds(log.stmts) {
				if kind == "user" {
					t.Error("the caller's SQL ran after the check failed")
				}
			}
			if log.rollbacks != 1 || log.commits != 0 {
				t.Errorf("rollbacks = %d, commits = %d, want 1 and 0", log.rollbacks, log.commits)
			}
		})
	}
}

func TestMySQLCatalogFallsBackToRoutinesOnMariaDB(t *testing.T) {
	log := &txLog{failOn: "user_defined_functions", version: "11.4.2-MariaDB"}
	sqlText := "SELECT UPPER(name) FROM employees"
	if _, err := runQuery(openFakeDB(t, log), classifyFor(t, EngineMySQL, sqlText), sqlText); err != nil {
		t.Fatalf("runQuery: %v", err)
	}
	if got, want := stmtKinds(log.stmts), []string{"lookup", "version", "lookup", "user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("statements = %v, want %v", got, want)
	}
	if strings.Contains(log.stmts[2], "user_defined_functions") {
		t.Errorf("MariaDB retry still reads the loadable-function table: %s", log.stmts[2])
	}
}

func TestMySQLCatalogUsesOnlyAllowlistedShapes(t *testing.T) {
	err := checkMySQLCatalog(nil, Statement{Catalog: CatalogNames{Functions: []string{"x') OR 1=1 -- "}}})
	var notAllowed *ErrFunctionNotAllowed
	if !errors.As(err, &notAllowed) {
		t.Errorf("checkMySQLCatalog with a malformed name = %v, want a refusal before any query", err)
	}
}

func TestRunWriteChecksBeforeTheStatement(t *testing.T) {
	cases := []struct {
		engine Engine
		want   []string
	}{
		{EngineMySQL, []string{"lookup", "user"}},
		{EnginePostgres, []string{"pins", "lookup", "user"}},
	}
	for _, c := range cases {
		t.Run(string(c.engine), func(t *testing.T) {
			log := &txLog{}
			sqlText := "UPDATE employees SET name = upper(name) WHERE id = 1"
			g := &Gate{}
			if _, err := g.runWrite("ch", "tester", classifyFor(t, c.engine, sqlText), openFakeDB(t, log), sqlText, false); err != nil {
				t.Fatalf("runWrite: %v", err)
			}
			if got := stmtKinds(log.stmts); !reflect.DeepEqual(got, c.want) {
				t.Errorf("statements = %v, want %v", got, c.want)
			}
			if !reflect.DeepEqual(log.readOnly, []bool{false}) {
				t.Errorf("BeginTx ReadOnly = %v, want [false]", log.readOnly)
			}
		})
	}
}

func TestRunWriteRefusesShadowedFunction(t *testing.T) {
	log := &txLog{shadowed: "upper"}
	sqlText := "UPDATE employees SET name = UPPER(name) WHERE id = 1"
	g := &Gate{}
	_, err := g.runWrite("ch", "tester", classifyFor(t, EngineMySQL, sqlText), openFakeDB(t, log), sqlText, true)
	var notAllowed *ErrFunctionNotAllowed
	if !errors.As(err, &notAllowed) {
		t.Fatalf("runWrite = %v, want a refusal", err)
	}
	if log.commits != 0 || log.rollbacks != 1 {
		t.Errorf("commits = %d, rollbacks = %d, want 0 and 1", log.commits, log.rollbacks)
	}
}

func TestPostgresTxGuardPinsCatalogAndStrings(t *testing.T) {
	for _, want := range []string{"'search_path'", "'pg_catalog'", "'standard_conforming_strings', 'on', true"} {
		if !strings.Contains(postgresTxGuard, want) {
			t.Errorf("postgresTxGuard lacks %s", want)
		}
	}
	if strings.Count(postgresTxGuard, ", true)") != 2 {
		t.Errorf("postgresTxGuard must set both values transaction-local: %s", postgresTxGuard)
	}
}

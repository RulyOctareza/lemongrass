// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClassifyQueryPostgresCatalogNames(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want CatalogNames
	}{
		{"nothing to look up", "SELECT 1", CatalogNames{}},
		{"bare call looked up", "SELECT lower(name) FROM employees", CatalogNames{Functions: []string{"lower"}}},
		{"pg_catalog call left out", "SELECT pg_catalog.lower(name) FROM employees", CatalogNames{}},
		{"parser-qualified syntax left out", "SELECT extract(year FROM now()), trim(name) FROM employees", CatalogNames{Functions: []string{"now"}}},
		{"attribute notation", "SELECT e.leak FROM employees e", CatalogNames{Attributes: []string{"leak"}}},
		{"indirection", "SELECT (e).leak FROM employees e", CatalogNames{Attributes: []string{"leak"}}},
		{"composite column field", "SELECT e.data.leak FROM employees e", CatalogNames{Attributes: []string{"data", "leak"}}},
		{"operators", "SELECT 1 FROM employees WHERE name = 'a' AND id > 1", CatalogNames{Operators: []string{"=", ">"}}},
		{"between maps to its operators", "SELECT 1 FROM employees WHERE id BETWEEN 1 AND 2", CatalogNames{Operators: []string{"<=", ">="}}},
		{"pg_catalog operator left out", "SELECT 1 FROM employees WHERE id OPERATOR(pg_catalog.=) 1", CatalogNames{}},
		{"sublink operator", "SELECT 1 FROM employees WHERE id = ANY (SELECT emp_id FROM salaries)", CatalogNames{Operators: []string{"="}}},
		{"write carries names", "UPDATE employees SET name = upper(name) WHERE id = 1", CatalogNames{Functions: []string{"upper"}, Operators: []string{"="}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ClassifyQuery(EnginePostgres, c.sql)
			if err != nil {
				t.Fatalf("ClassifyQuery: %v", err)
			}
			if !reflect.DeepEqual(got.Catalog, c.want) {
				t.Errorf("Catalog = %+v, want %+v", got.Catalog, c.want)
			}
		})
	}
}

func TestLookupPostgresCatalogSkipsEmptyNames(t *testing.T) {
	db, fake := openCatalogFake(t)
	if err := lookupPostgresCatalog(context.Background(), db, CatalogNames{}); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if n := fake.queries.Load(); n != 0 {
		t.Errorf("queries = %d, want 0", n)
	}
}

func TestLookupPostgresCatalogPassesNamesAsJSON(t *testing.T) {
	db, fake := openCatalogFake(t)
	names := CatalogNames{Functions: []string{"lower"}, Operators: []string{"="}}
	if err := lookupPostgresCatalog(context.Background(), db, names); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	want := []driver.Value{`["lower"]`, `[]`, `["="]`}
	if !reflect.DeepEqual(fake.args, want) {
		t.Errorf("args = %v, want %v", fake.args, want)
	}
}

func TestLookupPostgresCatalogRefusesFoundObject(t *testing.T) {
	cases := []struct {
		kind, name string
		want       string
	}{
		{"function", "lower", "dbgate: function lower is not allowed"},
		{"attribute", "leak", "dbgate: function leak is not allowed"},
		{"operator", "=", "dbgate: operator = is not allowed"},
	}
	for _, c := range cases {
		db, fake := openCatalogFake(t)
		fake.row = []driver.Value{c.kind, c.name}
		err := lookupPostgresCatalog(context.Background(), db, CatalogNames{Functions: []string{c.name}})
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: error = %v, want %q", c.kind, err, c.want)
		}
		var refused *ErrFunctionNotAllowed
		if c.kind != "operator" && !errors.As(err, &refused) {
			t.Errorf("%s: error %T is not *ErrFunctionNotAllowed", c.kind, err)
		}
	}
}

func TestLookupPostgresCatalogFailsClosed(t *testing.T) {
	db, fake := openCatalogFake(t)
	fake.err = errors.New("permission denied for table pg_proc")
	err := lookupPostgresCatalog(context.Background(), db, CatalogNames{Functions: []string{"lower"}})
	if err == nil || !strings.Contains(err.Error(), "the statement was not run") {
		t.Errorf("error = %v, want a refusal", err)
	}
}

// catalogFake answers the catalog query with at most one row, or an error, and records its arguments.
type catalogFake struct {
	queries atomic.Int32
	args    []driver.Value
	row     []driver.Value
	err     error
}

func openCatalogFake(t *testing.T) (*sql.DB, *catalogFake) {
	t.Helper()
	fake := &catalogFake{}
	db := sql.OpenDB(catalogFakeConnector{fake})
	t.Cleanup(func() { db.Close() })
	return db, fake
}

type catalogFakeConnector struct{ fake *catalogFake }

func (c catalogFakeConnector) Connect(context.Context) (driver.Conn, error) {
	return catalogFakeConn{c.fake}, nil
}

func (c catalogFakeConnector) Driver() driver.Driver { return nil }

type catalogFakeConn struct{ fake *catalogFake }

func (c catalogFakeConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (c catalogFakeConn) Close() error                        { return nil }
func (c catalogFakeConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

func (c catalogFakeConn) QueryContext(_ context.Context, _ string, args []driver.NamedValue) (driver.Rows, error) {
	c.fake.queries.Add(1)
	c.fake.args = nil
	for _, a := range args {
		c.fake.args = append(c.fake.args, a.Value)
	}
	if c.fake.err != nil {
		return nil, c.fake.err
	}
	return &catalogFakeRows{row: c.fake.row}, nil
}

type catalogFakeRows struct {
	row  []driver.Value
	done bool
}

func (r *catalogFakeRows) Columns() []string { return []string{"kind", "name"} }
func (r *catalogFakeRows) Close() error      { return nil }

func (r *catalogFakeRows) Next(dest []driver.Value) error {
	if r.done || r.row == nil {
		return io.EOF
	}
	r.done = true
	copy(dest, r.row)
	return nil
}

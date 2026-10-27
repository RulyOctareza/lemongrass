// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// pingTimeout bounds how long a connection test can block the vault's single-request-per-
// connection IPC handler.
const pingTimeout = 5 * time.Second

// pingWithTimeout checks db is actually reachable, not just that openDB parsed a well-formed
// connection string -- database/sql doesn't dial until first use, so this is the first real
// network round trip.
func pingWithTimeout(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("dbgate: connection test failed: %w", err)
	}
	return nil
}

// Engine identifies which database driver and SQL dialect a connection string names.
type Engine string

const (
	EngineMySQL    Engine = "mysql"
	EnginePostgres Engine = "postgres"
)

// engineOf reads the engine off a connection string's own scheme, per the book chapter's
// "engine folded into the connection string" decision -- mysql:// and mariadb:// both mean
// EngineMySQL, since MariaDB is wire-compatible and go-sql-driver/mysql serves both.
func engineOf(connString string) (Engine, error) {
	scheme, _, ok := strings.Cut(connString, "://")
	if !ok {
		return "", errors.New("dbgate: connection string has no scheme")
	}
	switch strings.ToLower(scheme) {
	case "mysql", "mariadb":
		return EngineMySQL, nil
	case "postgres", "postgresql":
		return EnginePostgres, nil
	default:
		return "", fmt.Errorf("dbgate: unsupported engine %q", scheme)
	}
}

// openDB opens a real database/sql connection for connString, picking the driver by scheme.
// The connection itself is lazy (database/sql doesn't dial until first use), so this only
// fails on a malformed connection string, not on the db actually being reachable.
func openDB(connString string) (*sql.DB, error) {
	engine, err := engineOf(connString)
	if err != nil {
		return nil, err
	}
	connString, sshAlias, err := splitSSHAlias(connString)
	if err != nil {
		return nil, err
	}
	switch engine {
	case EngineMySQL:
		dsn, err := mysqlDSN(connString)
		if err != nil {
			return nil, err
		}
		if sshAlias == "" {
			return sql.Open("mysql", dsn)
		}
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, fmt.Errorf("dbgate: parsing connection string: %w", err)
		}
		cfg.DialFunc = sshDialFunc(sshAlias)
		connector, err := mysql.NewConnector(cfg)
		if err != nil {
			return nil, fmt.Errorf("dbgate: parsing connection string: %w", err)
		}
		return sql.OpenDB(sshConnector{connector}), nil
	case EnginePostgres:
		if sshAlias == "" {
			return sql.Open("pgx", connString)
		}
		cfg, err := pgx.ParseConfig(connString)
		if err != nil {
			return nil, errors.New("dbgate: parsing connection string failed")
		}
		cfg.DialFunc = sshDialFunc(sshAlias)
		// The host name is resolved at the far end of the tunnel, where it may be the only place it exists.
		cfg.LookupFunc = func(_ context.Context, host string) ([]string, error) {
			return []string{host}, nil
		}
		return sql.OpenDB(sshConnector{stdlib.GetConnector(*cfg)}), nil
	default:
		return nil, fmt.Errorf("dbgate: unsupported engine %q", engine)
	}
}

// mysqlDSN translates a mysql://user:pass@host:port/db URL into go-sql-driver/mysql's own
// DSN shape (user:pass@tcp(host:port)/db), since that driver doesn't accept a URL directly.
func mysqlDSN(connString string) (string, error) {
	u, err := url.Parse(connString)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return "", fmt.Errorf("dbgate: parsing connection string: %w", err)
	}
	var userinfo string
	if u.User != nil {
		userinfo = u.User.String() + "@"
	}
	host := u.Host
	if host == "" {
		return "", errors.New("dbgate: connection string has no host")
	}
	db := strings.TrimPrefix(u.Path, "/")
	dsn := fmt.Sprintf("%stcp(%s)/%s", userinfo, host, db)
	if u.RawQuery != "" {
		dsn += "?" + u.RawQuery
	}
	return dsn, nil
}

// listTablesQuery returns the engine-appropriate information_schema query for enumerating the
// connection's own database/schema, so table names come from what's actually there rather than
// free-typed guesses.
func listTablesQuery(engine Engine) (string, error) {
	switch engine {
	case EngineMySQL:
		return "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name", nil
	case EnginePostgres:
		return "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() ORDER BY table_name", nil
	default:
		return "", fmt.Errorf("dbgate: unsupported engine %q", engine)
	}
}

// listTables runs engine's own listTablesQuery against db and returns the table names found.
func listTables(db *sql.DB, engine Engine) ([]string, error) {
	query, err := listTablesQuery(engine)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("dbgate: listing tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("dbgate: scanning table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dbgate: reading table names: %w", err)
	}
	return tables, nil
}

// QueryResult is a read-only statement's shaped output -- the only thing that ever leaves the
// vault for a query, never the credential that produced it.
type QueryResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

// runQuery executes sqlText against db and shapes the result. sqlText has already been
// classified as a single read-only statement by ClassifyQuery before this is ever called.
func runQuery(db *sql.DB, sqlText string) (QueryResult, error) {
	rows, err := db.Query(sqlText)
	if err != nil {
		return QueryResult{}, fmt.Errorf("dbgate: executing query: %s", describeDBError(err))
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return QueryResult{}, fmt.Errorf("dbgate: reading columns: %w", err)
	}

	result := QueryResult{Columns: columns, Rows: [][]interface{}{}}
	for rows.Next() {
		raw := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return QueryResult{}, fmt.Errorf("dbgate: scanning row: %w", err)
		}
		row := make([]interface{}, len(raw))
		for i, v := range raw {
			row[i] = normalizeValue(v)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return QueryResult{}, fmt.Errorf("dbgate: reading rows: %w", err)
	}
	return result, nil
}

// normalizeValue converts a driver-returned column value into something JSON-safe: nil,
// numbers, strings, and booleans pass through as-is; time.Time becomes RFC3339; anything
// else -- binary columns included -- becomes a string.
func normalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case nil, bool, int64, float64, string:
		return val
	case time.Time:
		return val.Format(time.RFC3339)
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

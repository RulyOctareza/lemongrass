// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestMySQLConfigForcesOneStatementAndNoFiles(t *testing.T) {
	for _, conn := range []string{
		"mysql://app:pw@db:3306/app?multiStatements=true&allowAllFiles=true",
		"mysql://app:pw@db:3306/app?multiStatements=true&allowAllFiles=true&lgrass_ssh=bastion",
	} {
		plain, _, err := splitSSHAlias(conn)
		if err != nil {
			t.Fatalf("splitSSHAlias(%q): %v", conn, err)
		}
		cfg, err := mysqlConfig(plain)
		if err != nil {
			t.Fatalf("mysqlConfig(%q): %v", conn, err)
		}
		if cfg.MultiStatements {
			t.Errorf("%q: MultiStatements = true, want false", conn)
		}
		if cfg.AllowAllFiles {
			t.Errorf("%q: AllowAllFiles = true, want false", conn)
		}
	}
}

func TestMySQLConfigDropsRawParamsAndForcesCharset(t *testing.T) {
	cfg, err := mysqlConfig("mysql://app:pw@db:3306/app?sql_mode=ANSI_QUOTES&SQL_MODE=ANSI&%40%40session.sql_mode=NO_BACKSLASH_ESCAPES&character_set_client=gbk&charset=gbk&collation=gbk_chinese_ci&time_zone=%27%2B00%3A00%27&parseTime=true&timeout=5s")
	if err != nil {
		t.Fatalf("mysqlConfig: %v", err)
	}
	if want := map[string]string{"sql_mode": mysqlSQLModeExpr()}; !reflect.DeepEqual(cfg.Params, want) {
		t.Errorf("Params = %v, want only the forced sql_mode", cfg.Params)
	}
	dsn := cfg.FormatDSN()
	if !strings.Contains(dsn, "charset=utf8mb4") || strings.Contains(dsn, "gbk") {
		t.Errorf("charset not forced to utf8mb4: %s", dsn)
	}
	if !cfg.ParseTime || cfg.Timeout.String() != "5s" {
		t.Errorf("driver options were dropped: parseTime=%v timeout=%v", cfg.ParseTime, cfg.Timeout)
	}
}

func TestPostgresConfigForcesUTF8(t *testing.T) {
	cfg, err := postgresConfig("postgres://app:pw@db:5432/app?client_encoding=SJIS")
	if err != nil {
		t.Fatalf("postgresConfig: %v", err)
	}
	if got := cfg.RuntimeParams["client_encoding"]; got != "UTF8" {
		t.Errorf("client_encoding = %q, want UTF8", got)
	}
}

func TestMySQLSQLModeExprStripsLexModes(t *testing.T) {
	expr := mysqlSQLModeExpr()
	if !strings.Contains(expr, "@@SESSION.sql_mode") {
		t.Errorf("expression doesn't start from the session's own mode: %s", expr)
	}
	for _, mode := range []string{"NO_BACKSLASH_ESCAPES", "ANSI_QUOTES", "ANSI", "ORACLE", "MSSQL", "DB2", "MAXDB", "POSTGRESQL"} {
		if !strings.Contains(expr, "',"+mode+",'") {
			t.Errorf("expression doesn't strip %s: %s", mode, expr)
		}
	}
}

func TestPostgresConfigNeverUsesSimpleProtocol(t *testing.T) {
	cases := []struct {
		conn string
		want pgx.QueryExecMode
	}{
		{"postgres://app:pw@db:5432/app?default_query_exec_mode=simple_protocol", pgx.QueryExecModeCacheStatement},
		{"postgres://app:pw@db:5432/app?default_query_exec_mode=exec", pgx.QueryExecModeExec},
		{"postgres://app:pw@db:5432/app?default_query_exec_mode=describe_exec", pgx.QueryExecModeDescribeExec},
		{"postgres://app:pw@db:5432/app", pgx.QueryExecModeCacheStatement},
	}
	for _, c := range cases {
		cfg, err := postgresConfig(c.conn)
		if err != nil {
			t.Fatalf("postgresConfig(%q): %v", c.conn, err)
		}
		if cfg.DefaultQueryExecMode != c.want {
			t.Errorf("%q: DefaultQueryExecMode = %v, want %v", c.conn, cfg.DefaultQueryExecMode, c.want)
		}
	}
}

func TestPostgresConfigErrorHidesConnectionString(t *testing.T) {
	_, err := postgresConfig("postgres://app:secretpw@db:notaport/app")
	if err == nil {
		t.Fatal("postgresConfig: expected an error, got nil")
	}
	if strings.Contains(err.Error(), "secretpw") {
		t.Errorf("error echoes the password: %v", err)
	}
}

func TestOpenDBPlainPathsOpenThroughConnector(t *testing.T) {
	for _, conn := range []string{
		"mysql://app:pw@db:3306/app?multiStatements=true",
		"postgres://app:pw@db:5432/app?default_query_exec_mode=simple_protocol",
	} {
		db, err := openDB(conn)
		if err != nil {
			t.Fatalf("openDB(%q): %v", conn, err)
		}
		db.Close()
	}
}

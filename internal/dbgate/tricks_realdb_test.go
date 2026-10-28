// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/vault"
)

// These tests need the servers from scripts/dbgate-realdb, connected as its least-privilege app user. They run the trick catalog against real MySQL and Postgres and check afterwards that nothing in the database changed.

func TestTricksAgainstRealMySQL(t *testing.T) {
	dsn := os.Getenv("LGRASS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("LGRASS_TEST_MYSQL_DSN not set, skipping real-MySQL test")
	}
	runTrickSuite(t, EngineMySQL, dsn, mysqlTricks)
}

func TestTricksAgainstRealPostgres(t *testing.T) {
	dsn := os.Getenv("LGRASS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("LGRASS_TEST_POSTGRES_DSN not set, skipping real-Postgres test")
	}
	runTrickSuite(t, EnginePostgres, dsn, postgresTricks)
}

func runTrickSuite(t *testing.T, engine Engine, dsn string, tricks []trick) {
	observer := openRealDB(t, dsn)
	before := snapshotState(t, observer, engine)
	baseline := sessionState(t, openSingleConn(t, dsn), engine)

	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "trick-db", []byte(dsn)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	channels := newTrickChannels(t, svc, dsn)

	t.Run("gate", func(t *testing.T) {
		for _, tr := range tricks {
			t.Run(tr.name, func(t *testing.T) {
				id := channels.get(tr.scope)
				var err error
				if tr.path == trickWrite {
					_, err = svc.Execute(id, tr.sql, true, "red-team")
				} else {
					_, err = svc.Query(id, tr.sql)
				}
				if err == nil {
					t.Errorf("trick was allowed: %q", tr.sql)
					return
				}
				t.Logf("refused: %v", err)
			})
		}
	})

	t.Run("controls", func(t *testing.T) {
		for _, c := range trickControls[engine] {
			t.Run(c.sql, func(t *testing.T) {
				id := channels.get(c.scope)
				var err error
				if c.path == trickWrite {
					// A dry run, so the snapshot at the end still has to match.
					_, err = svc.Execute(id, c.sql, false, "red-team")
				} else {
					_, err = svc.Query(id, c.sql)
				}
				if err != nil {
					t.Errorf("ordinary %s was refused: %v", c.path, err)
				}
			})
		}
	})

	t.Run("read-only transaction alone", func(t *testing.T) {
		db := openSingleConn(t, dsn)
		for _, tr := range tricks {
			if !tr.dbStops {
				continue
			}
			t.Run(tr.name, func(t *testing.T) {
				_, err := runQuery(db, Statement{Engine: engine}, tr.sql)
				if err == nil || !strings.Contains(err.Error(), readOnlyErrorCode(engine)) {
					t.Errorf("runQuery(%q) = %v, want the database's read-only refusal", tr.sql, err)
				}
			})
		}
		sessionUnchanged(t, db, engine, baseline)
	})

	t.Run("session state rolls back", func(t *testing.T) {
		db := openSingleConn(t, dsn)
		for _, q := range sessionChangingReads(engine) {
			// Each may succeed: the claim is that nothing it set outlives the call.
			if _, err := runQuery(db, Statement{Engine: engine}, q); err != nil {
				t.Logf("runQuery(%q): %v", q, err)
			}
		}
		sessionUnchanged(t, db, engine, baseline)
	})

	t.Run("one statement per call", func(t *testing.T) {
		for _, param := range stackingParams(engine) {
			db := openSingleConn(t, withParam(dsn, param))
			for _, q := range []string{
				"SELECT 1; DELETE FROM employees WHERE id = 1",
				"SELECT 1; UPDATE salaries SET amount = 0 WHERE employee_id = 1",
			} {
				if _, err := runQuery(db, Statement{Engine: engine}, q); err == nil {
					t.Errorf("with %s, runQuery(%q) ran stacked statements", param, q)
				}
			}
		}
	})

	t.Run("forced session settings", func(t *testing.T) {
		for param, check := range forcedSettings(engine) {
			db := openSingleConn(t, withParam(dsn, param))
			check(t, db)
		}
	})

	t.Run("statement time limit", func(t *testing.T) {
		old := statementLimit
		statementLimit = 2 * time.Second
		t.Cleanup(func() { statementLimit = old })
		id := channels.get(controlWrite)

		cases := []struct {
			name string
			run  func(db *sql.DB) error
		}{
			{"long read", func(db *sql.DB) error {
				_, err := runQuery(db, Statement{Engine: engine}, longSleepRead(engine))
				return err
			}},
			{"long write", func(db *sql.DB) error {
				_, err := svc.Gate.runWrite(id, "red-team", Statement{Engine: engine}, db, longSleepWrite(engine), false)
				return err
			}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				// A fresh single-connection pool, from openDB so the MySQL killer is registered.
				db := openSingleConn(t, dsn)
				start := time.Now()
				err := c.run(db)
				elapsed := time.Since(start)
				if err == nil || !strings.Contains(err.Error(), "dbgate: the statement ran past the") {
					t.Fatalf("%s: got %v, want the statement-limit error", c.name, err)
				}
				// The server-side stop fires near statementLimit; running to the client backstop means it did not.
				if elapsed > statementLimit+6*time.Second {
					t.Errorf("%s ran %v, the server-side stop did not fire before the client backstop", c.name, elapsed)
				}
				// A killed connection is left so the next pooled query runs normally.
				if _, err := runQuery(db, Statement{Engine: engine}, "SELECT 1"); err != nil {
					t.Errorf("next query after %s failed: %v", c.name, err)
				}
			})
		}
	})

	for _, db := range channels.pools() {
		sessionUnchanged(t, db, engine, baseline)
	}
	after := snapshotState(t, observer, engine)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("database state changed:\nbefore %+v\nafter  %+v", before, after)
	}
}

// trickChannels creates one channel per distinct scope, each with a pool held to one connection so a session setting a trick leaves behind is seen by the next query on it.
type trickChannels struct {
	t   *testing.T
	svc *testSvc
	dsn string
	ids map[string]vault.ChannelID
}

func newTrickChannels(t *testing.T, svc *testSvc, dsn string) *trickChannels {
	return &trickChannels{t: t, svc: svc, dsn: dsn, ids: make(map[string]vault.ChannelID)}
}

func (c *trickChannels) get(s vault.Scope) vault.ChannelID {
	key := fmt.Sprint(s.Tables, s.Operations)
	if id, ok := c.ids[key]; ok {
		return id
	}
	ch, err := c.svc.CreateChannel(testRootSecret, "trick-"+fmt.Sprint(len(c.ids)), "trick-db", s, 10*time.Minute)
	if err != nil {
		c.t.Fatalf("CreateChannel: %v", err)
	}
	db, err := c.svc.Gate.getDB(ch.ID, c.dsn)
	if err != nil {
		c.t.Fatalf("getDB: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	c.ids[key] = ch.ID
	return ch.ID
}

func (c *trickChannels) pools() []*sql.DB {
	var out []*sql.DB
	for _, id := range c.ids {
		db, err := c.svc.Gate.getDB(id, c.dsn)
		if err != nil {
			c.t.Fatalf("getDB: %v", err)
		}
		out = append(out, db)
	}
	return out
}

func openRealDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func openSingleConn(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db := openRealDB(t, dsn)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db
}

func withParam(dsn, param string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&" + param
	}
	return dsn + "?" + param
}

// readOnlyErrorCode is the server error each engine raises for a write inside a read-only transaction, as describeDBError prints it.
func readOnlyErrorCode(engine Engine) string {
	if engine == EngineMySQL {
		return "database error 1792"
	}
	return "database error 25006"
}

// dbState is what a trick must leave unchanged: every relation, every row, the sequence, the user functions and the locks other sessions hold.
type dbState struct {
	Relations []string
	Checksums map[string]string
	Sequence  string
	Functions []string
	Locks     string
}

func snapshotState(t *testing.T, db *sql.DB, engine Engine) dbState {
	t.Helper()
	var s dbState
	if engine == EngineMySQL {
		s.Relations = queryStrings(t, db, "SELECT CONCAT(table_schema, '.', table_name, ':', table_type) FROM information_schema.tables WHERE table_schema IN ('vaultapp', 'vault_archive') ORDER BY 1")
		s.Checksums = make(map[string]string)
		for _, name := range queryStrings(t, db, "SELECT CONCAT(table_schema, '.', table_name) FROM information_schema.tables WHERE table_schema IN ('vaultapp', 'vault_archive') AND table_type = 'BASE TABLE' ORDER BY 1") {
			var table, sum sql.NullString
			if err := db.QueryRow("CHECKSUM TABLE "+name+" EXTENDED").Scan(&table, &sum); err != nil {
				t.Fatalf("checksum %s: %v", name, err)
			}
			s.Checksums[name] = sum.String
		}
		s.Sequence = queryString(t, db, "SELECT CAST(next_value AS CHAR) FROM vaultapp.invoice_counter WHERE id = 1")
		s.Functions = queryStrings(t, db, "SELECT CONCAT(routine_schema, '.', routine_name) FROM information_schema.routines WHERE routine_schema IN ('vaultapp', 'vault_archive') ORDER BY 1")
		s.Locks = queryString(t, db, "SELECT CAST((SELECT COUNT(*) FROM performance_schema.metadata_locks WHERE owner_thread_id <> PS_CURRENT_THREAD_ID() AND (object_type = 'USER LEVEL LOCK' OR object_schema IN ('vaultapp', 'vault_archive'))) + (SELECT COUNT(*) FROM performance_schema.data_locks WHERE object_schema IN ('vaultapp', 'vault_archive')) AS CHAR)")
		return s
	}
	s.Relations = queryStrings(t, db, "SELECT n.nspname || '.' || c.relname || ':' || c.relkind::text FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname IN ('public', 'archive') OR n.nspname LIKE 'pg\\_temp\\_%' ORDER BY 1")
	s.Checksums = make(map[string]string)
	for _, name := range queryStrings(t, db, "SELECT schemaname || '.' || tablename FROM pg_tables WHERE schemaname IN ('public', 'archive') ORDER BY 1") {
		s.Checksums[name] = queryString(t, db, "SELECT md5(coalesce(string_agg(t::text, '|' ORDER BY t::text), '')) FROM "+name+" t")
	}
	s.Sequence = queryString(t, db, "SELECT last_value::text || ':' || is_called::text FROM invoice_seq")
	s.Functions = queryStrings(t, db, "SELECT n.nspname || '.' || p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname IN ('public', 'archive') ORDER BY 1")
	s.Locks = queryString(t, db, "SELECT count(*)::text FROM pg_locks l WHERE l.pid <> pg_backend_pid() AND (l.locktype = 'advisory' OR l.relation IN (SELECT c.oid FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname IN ('public', 'archive')))")
	return s
}

// sessionState reads, outside any transaction, the settings the vault pins and the state a read could try to leave on a pooled connection.
func sessionState(t *testing.T, db *sql.DB, engine Engine) []string {
	t.Helper()
	if engine == EngineMySQL {
		return queryRow(t, db, "SELECT @@SESSION.sql_mode, @@SESSION.character_set_client, CAST(@@SESSION.autocommit AS CHAR), CAST(@@SESSION.transaction_read_only AS CHAR), "+
			"CAST((SELECT COUNT(*) FROM performance_schema.user_variables_by_thread WHERE thread_id = PS_CURRENT_THREAD_ID()) AS CHAR), "+
			"CAST((SELECT COUNT(*) FROM performance_schema.metadata_locks WHERE owner_thread_id = PS_CURRENT_THREAD_ID() AND object_type = 'USER LEVEL LOCK') AS CHAR)", 6)
	}
	return queryRow(t, db, "SELECT current_setting('search_path'), current_setting('standard_conforming_strings'), current_setting('client_encoding'), current_setting('transaction_read_only'), "+
		"(SELECT count(*)::text FROM pg_locks WHERE pid = pg_backend_pid() AND locktype = 'advisory'), "+
		"(SELECT count(*)::text FROM pg_class WHERE relnamespace = pg_my_temp_schema())", 6)
}

func sessionUnchanged(t *testing.T, db *sql.DB, engine Engine, baseline []string) {
	t.Helper()
	if got := sessionState(t, db, engine); !reflect.DeepEqual(got, baseline) {
		t.Errorf("pooled session state changed: got %q, want %q", got, baseline)
	}
}

// sessionChangingReads would each leave a setting on the connection if anything they did outlived the read-only transaction's rollback.
func sessionChangingReads(engine Engine) []string {
	if engine == EngineMySQL {
		// A MySQL read-only transaction does not undo user variables or GET_LOCK, so only the classifier keeps those out; tricks_test.go pins them.
		return nil
	}
	return []string{
		"SELECT set_config('search_path', 'archive', false)",
		"SELECT set_config('standard_conforming_strings', 'off', false)",
		"SELECT pg_advisory_xact_lock(4242)",
	}
}

// stackingParams are connection-string settings that would let the driver send several statements in one call if the vault did not override them.
func stackingParams(engine Engine) []string {
	if engine == EngineMySQL {
		return []string{"multiStatements=true", "multiStatements=true&interpolateParams=true"}
	}
	return []string{"default_query_exec_mode=simple_protocol", "default_query_exec_mode=exec"}
}

// forcedSettings maps a connection-string parameter the vault must not honor to a check of the session it actually gets.
func forcedSettings(engine Engine) map[string]func(*testing.T, *sql.DB) {
	if engine == EngineMySQL {
		noLexModes := func(t *testing.T, db *sql.DB) {
			mode := queryString(t, db, "SELECT @@SESSION.sql_mode")
			for _, m := range []string{"ANSI_QUOTES", "NO_BACKSLASH_ESCAPES"} {
				if strings.Contains(mode, m) {
					t.Errorf("sql_mode %q still holds %s", mode, m)
				}
			}
		}
		utf8 := func(t *testing.T, db *sql.DB) {
			if got := queryString(t, db, "SELECT @@SESSION.character_set_client"); got != "utf8mb4" {
				t.Errorf("character_set_client = %q, want utf8mb4", got)
			}
		}
		return map[string]func(*testing.T, *sql.DB){
			"sql_mode=%27ANSI_QUOTES%27":              noLexModes,
			"sql_mode=%27NO_BACKSLASH_ESCAPES%27":     noLexModes,
			"@@session.sql_mode=%27ANSI_QUOTES%27":    noLexModes,
			"charset=gbk":                             utf8,
			"character_set_client=gbk":                utf8,
			"charset=sjis&collation=sjis_japanese_ci": utf8,
			"autocommit=0&transaction_read_only=0":    func(t *testing.T, db *sql.DB) { utf8(t, db); noLexModes(t, db) },
		}
	}
	utf8 := func(t *testing.T, db *sql.DB) {
		if got := queryString(t, db, "SELECT current_setting('client_encoding')"); got != "UTF8" {
			t.Errorf("client_encoding = %q, want UTF8", got)
		}
	}
	return map[string]func(*testing.T, *sql.DB){
		"client_encoding=SJIS":                utf8,
		"options=-c%20client_encoding%3DBIG5": utf8,
		"options=-c%20standard_conforming_strings%3Doff": func(t *testing.T, db *sql.DB) {
			got, err := runQuery(db, Statement{Engine: EnginePostgres}, "SELECT current_setting('standard_conforming_strings')")
			if err != nil {
				t.Fatalf("runQuery: %v", err)
			}
			if v := got.Rows[0][0]; v != "on" {
				t.Errorf("standard_conforming_strings inside the read = %v, want on", v)
			}
		},
		"options=-c%20search_path%3Darchive%2Cpg_catalog": func(t *testing.T, db *sql.DB) {
			got, err := runQuery(db, Statement{Engine: EnginePostgres}, "SELECT current_setting('search_path')")
			if err != nil {
				t.Fatalf("runQuery: %v", err)
			}
			if v, _ := got.Rows[0][0].(string); !strings.HasPrefix(v, "pg_catalog") {
				t.Errorf("search_path inside the read = %q, want pg_catalog first", v)
			}
		},
	}
}

func queryStrings(t *testing.T, db *sql.DB, q string) []string {
	t.Helper()
	rows, err := db.Query(q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		out = append(out, v.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return out
}

func queryString(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	return queryRow(t, db, q, 1)[0]
}

func queryRow(t *testing.T, db *sql.DB, q string, n int) []string {
	t.Helper()
	vals := make([]sql.NullString, n)
	ptrs := make([]any, n)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := db.QueryRow(q).Scan(ptrs...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	out := make([]string, n)
	for i, v := range vals {
		out[i] = v.String
	}
	return out
}

// longSleepRead is a read that runs well past the lowered statementLimit, run through runQuery with the classifier skipped since the allowlist bars the sleep functions.
func longSleepRead(engine Engine) string {
	if engine == EngineMySQL {
		return "SELECT SLEEP(60)"
	}
	return "SELECT pg_sleep(60)"
}

// longSleepWrite is a write that runs past the limit, to prove the bound covers writes too and not only MySQL's SELECT-only max_execution_time.
func longSleepWrite(engine Engine) string {
	if engine == EngineMySQL {
		return "UPDATE employees SET name = name WHERE id = 1 AND SLEEP(60) = 0"
	}
	return "UPDATE employees e SET name = name FROM pg_sleep(60) s WHERE e.id = 1"
}

// trickControl is an ordinary statement the vault must still run, so the suite proves it refuses tricks rather than everything.
type trickControl struct {
	sql   string
	scope vault.Scope
	path  trickPath
}

var (
	controlRead  = vault.Scope{Tables: []string{"employees", "salaries"}, Operations: []string{"select", "explain", "show"}}
	controlPerf  = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", OperationPerformance}}
	controlWrite = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", "insert", "update", "delete"}}
)

// trickControls avoid the names the seeds trap on purpose: on Postgres a bare = and lower resolve to the seeded public overloads, so comparisons use > or the pg_catalog form, and on MySQL sum is a seeded stored function.
var trickControls = map[Engine][]trickControl{
	EnginePostgres: {
		{"SELECT e.id, e.name, s.amount FROM employees e JOIN salaries s ON e.id OPERATOR(pg_catalog.=) s.employee_id ORDER BY e.id", controlRead, trickRead},
		{"SELECT count(*), avg(amount), max(amount) FROM salaries", controlRead, trickRead},
		{"SELECT upper(name), length(name), date_trunc('day', now()), to_char(now(), 'YYYY') FROM employees", controlRead, trickRead},
		{"SELECT pg_catalog.lower(name) FROM employees", controlRead, trickRead},
		{"SELECT id FROM employees WHERE name OPERATOR(pg_catalog.=) 'Ada Example'", controlRead, trickRead},
		{"WITH top AS (SELECT employee_id FROM salaries WHERE amount > 5500) SELECT count(*) FROM top", controlRead, trickRead},
		{"EXPLAIN SELECT * FROM employees WHERE id > 1", controlRead, trickRead},
		{"SELECT relname, seq_scan FROM pg_stat_user_tables", controlPerf, trickRead},
		{"SELECT calls, total_exec_time FROM pg_stat_statements LIMIT 5", controlPerf, trickRead},
		{"INSERT INTO employees (id, name) VALUES (900, 'Dee Draft')", controlWrite, trickWrite},
		{"UPDATE employees SET name = upper(name) WHERE id > 2", controlWrite, trickWrite},
		{"DELETE FROM employees WHERE id > 2", controlWrite, trickWrite},
	},
	EngineMySQL: {
		{"SELECT e.id, e.name, s.amount FROM employees e JOIN salaries s ON e.id = s.employee_id ORDER BY e.id", controlRead, trickRead},
		{"SELECT COUNT(*), AVG(amount), MAX(amount) FROM salaries", controlRead, trickRead},
		{"SELECT UPPER(name), LENGTH(name), DATE_FORMAT(NOW(), '%Y'), CONCAT(name, '!') FROM employees", controlRead, trickRead},
		{"SELECT id FROM employees WHERE name = 'Ada Example'", controlRead, trickRead},
		{"SELECT NOW()", controlRead, trickRead},
		{"SELECT 1", controlRead, trickRead},
		{"SELECT JSON_EXTRACT('{\"a\":1}', '$.a')", controlRead, trickRead},
		{"SELECT NOW() FROM dual", controlRead, trickRead},
		{"EXPLAIN SELECT * FROM employees WHERE id > 1", controlRead, trickRead},
		{"DESCRIBE employees", controlRead, trickRead},
		{"SHOW TABLES", controlRead, trickRead},
		{"SELECT digest_text, count_star FROM performance_schema.events_statements_summary_by_digest LIMIT 5", controlPerf, trickRead},
		{"INSERT INTO employees (id, name) VALUES (900, 'Dee Draft')", controlWrite, trickWrite},
		{"UPDATE employees SET name = UPPER(name) WHERE id > 2", controlWrite, trickWrite},
		{"DELETE FROM employees WHERE id > 2", controlWrite, trickWrite},
	},
}

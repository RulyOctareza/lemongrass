// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// mysqlStayOut are functions with side effects or reach beyond the statement's tables, which must never join the allowlist.
var mysqlStayOut = []string{
	"sleep", "benchmark", "get_lock", "release_lock", "release_all_locks", "is_free_lock",
	"is_used_lock", "load_file", "last_insert_id", "uuid_short", "source_pos_wait",
	"master_pos_wait", "wait_for_executed_gtid_set", "statement_digest", "extractvalue", "updatexml",
}

func TestMySQLAllowlistShape(t *testing.T) {
	for name := range mysqlAllowedFunctions {
		if name != strings.ToLower(name) {
			t.Errorf("allowlisted %q is not lowercase, and MySQL names are matched lowered", name)
		}
	}
	for _, name := range mysqlStayOut {
		if mysqlAllowedFunctions[name] {
			t.Errorf("%s is on the MySQL allowlist", name)
		}
	}
	// Node forms are checked by type; a FuncExpr with one of these names only comes from a backticked call to a stored function.
	for _, name := range []string{"cast", "convert", "group_concat", "substr", "substring", "match", "values", "default"} {
		if mysqlAllowedFunctions[name] {
			t.Errorf("node form %s must not be on the name allowlist", name)
		}
	}
}

func TestMySQLAllowedCalls(t *testing.T) {
	for _, sql := range []string{
		"SELECT COUNT(*), SUM(salary), AVG(salary), MAX(id) FROM employees",
		"SELECT CONCAT(first_name, ' ', last_name), UPPER(email), LEFT(name, 1), RIGHT(name, 2) FROM employees",
		"SELECT CAST(id AS CHAR), CONVERT(name, CHAR), CONVERT(name USING utf8mb4) FROM employees",
		"SELECT GROUP_CONCAT(DISTINCT name ORDER BY name SEPARATOR ',') FROM employees",
		"SELECT SUBSTRING(name, 1, 3), SUBSTR(name, 2) FROM employees",
		"SELECT id FROM employees WHERE MATCH(name) AGAINST ('alice')",
		"SELECT CASE WHEN id > 1 THEN 'a' ELSE 'b' END, IF(id > 1, 1, 0), IFNULL(name, ''), COALESCE(name, email) FROM employees",
		"SELECT DATE_ADD(hired, INTERVAL 1 DAY), DATE_FORMAT(hired, '%Y'), NOW(), CURRENT_TIMESTAMP FROM employees",
		"SELECT JSON_EXTRACT(meta, '$.a'), meta->'$.b', meta->>'$.c' FROM employees",
		"SELECT ROUND(salary, 2), MOD(id, 2), REPLACE(name, 'a', 'b'), DATABASE() FROM employees",
		"SELECT `sum` FROM employees",
		"SELECT @x, @@version",
		"SELECT format_pico_time(sum_timer_wait), format_bytes(sum_number_of_bytes_alloc) FROM performance_schema.events_statements_summary_by_digest",
		"SELECT SlEeP_count FROM employees",
		"SELECT 'sum (x)' FROM employees",
		"EXPLAIN SELECT COUNT(*) FROM employees",
		"INSERT INTO employees (id, name, hired) VALUES (1, UPPER('a'), NOW()) ON DUPLICATE KEY UPDATE name = VALUES(name)",
		"UPDATE employees SET name = CONCAT(name, 'x') WHERE id = 1",
		"DELETE FROM employees WHERE hired < DATE_SUB(NOW(), INTERVAL 1 YEAR)",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

func TestMySQLStayOutFunctionsRefusedEverywhere(t *testing.T) {
	positions := []string{
		"SELECT %s FROM employees",
		"SELECT id FROM employees WHERE %s",
		"SELECT id FROM employees GROUP BY id HAVING %s",
		"SELECT id FROM employees ORDER BY %s",
		"SELECT e.id FROM employees e JOIN employees f ON %s",
		"SELECT * FROM (SELECT %s) d",
		"SELECT id FROM employees WHERE id = (SELECT %s)",
		"SELECT id FROM employees WHERE EXISTS (SELECT %s)",
		"SELECT GROUP_CONCAT(%s) FROM employees",
		"SELECT CAST(%s AS CHAR) FROM employees",
		"SELECT CASE WHEN %s THEN 1 END FROM employees",
		"SELECT CONCAT(UPPER(%s)) FROM employees",
		"SELECT id FROM employees UNION SELECT id FROM employees ORDER BY %s",
		"EXPLAIN SELECT %s FROM employees",
		"EXPLAIN ANALYZE SELECT %s FROM employees",
		"SHOW TABLES WHERE %s",
		"INSERT INTO employees (id) VALUES (%s)",
		"INSERT INTO employees (id) SELECT %s",
		"INSERT INTO employees (id) VALUES (1) ON DUPLICATE KEY UPDATE id = %s",
		"UPDATE employees SET id = %s WHERE id = 1",
		"UPDATE employees SET id = 1 WHERE %s",
		"DELETE FROM employees WHERE %s",
	}
	for _, name := range mysqlStayOut {
		call := name + "(1)"
		for _, pos := range positions {
			sql := strings.Replace(pos, "%s", call, 1)
			_, err := ClassifyQuery(EngineMySQL, sql)
			var notAllowed *ErrFunctionNotAllowed
			if !errors.As(err, &notAllowed) {
				t.Errorf("ClassifyQuery(%q) = %v, want ErrFunctionNotAllowed", sql, err)
				continue
			}
			if notAllowed.Name != name {
				t.Errorf("ClassifyQuery(%q) refused %q, want %q", sql, notAllowed.Name, name)
			}
		}
	}
}

func TestMySQLFunctionNameForms(t *testing.T) {
	cases := []struct {
		sql  string
		name string
	}{
		{"SELECT SLEEP(1)", "sleep"},
		{"SELECT SlEeP(1)", "sleep"},
		{"SELECT `sleep`(1)", "sleep"},
		{"SELECT SLEEP/**/(1)", "sleep"},
		{"SELECT /*! SLEEP(1) */", "sleep"},
		{"SELECT 1 /*!50000 , SLEEP(1) */", "sleep"},
		{"SELECT appdb.leak()", "appdb.leak"},
		{"SELECT `appdb`.`concat`('a')", "appdb.concat"},
		{"SELECT sys.format_time(1)", "sys.format_time"},
		{"SELECT my_udf(id) FROM employees", "my_udf"},
		{"SELECT `group_concat`(id) FROM employees", "group_concat"},
	}
	for _, c := range cases {
		_, err := ClassifyQuery(EngineMySQL, c.sql)
		var notAllowed *ErrFunctionNotAllowed
		if !errors.As(err, &notAllowed) || notAllowed.Name != c.name {
			t.Errorf("ClassifyQuery(%q) = %v, want function %s refused", c.sql, err, c.name)
			continue
		}
		if want := "dbgate: function " + c.name + " is not allowed"; err.Error() != want {
			t.Errorf("ClassifyQuery(%q) message = %q, want %q", c.sql, err.Error(), want)
		}
	}
}

func TestMySQLCallSpellingRefused(t *testing.T) {
	for _, sql := range []string{
		"SELECT sum (salary) FROM employees",
		"SELECT SUM\t(salary) FROM employees",
		"SELECT sum/**/(salary) FROM employees",
		"SELECT sum -- x\n(salary) FROM employees",
		"SELECT count # x\n(*) FROM employees",
		"SELECT `count`(*) FROM employees",
		"SELECT `concat` ('a', 'b')",
		"SELECT SUBSTRING (name, 1) FROM employees",
		"SELECT CAST (id AS CHAR) FROM employees",
		"SELECT if (id, 1, 0) FROM employees",
		"UPDATE employees SET name = concat (name, 'x') WHERE id = 1",
	} {
		_, err := ClassifyQuery(EngineMySQL, sql)
		if err == nil || !strings.Contains(err.Error(), "must be written unquoted") {
			t.Errorf("ClassifyQuery(%q) = %v, want the spelling refusal", sql, err)
		}
	}
}

func TestMySQLCommentDesyncRefused(t *testing.T) {
	cases := []struct {
		sql  string
		want error
	}{
		{"SELECT 1 --1, (SELECT secret FROM salaries)", errMySQLDashComment},
		{"SELECT id FROM employees --x", errMySQLDashComment},
		{"SELECT id FROM employees WHERE id = 1 --", errMySQLDashComment},
		{"SELECT 1 /*! --1, SLEEP(1) */", errMySQLDashComment},
		{"SELECT 1 //, (SELECT secret FROM salaries)", errMySQLSlashComment},
		{"SELECT /*+ SET_VAR(sort_buffer_size = 16M) */ id FROM employees", errMySQLHint},
		{"SELECT 1 /*M! , (SELECT secret FROM salaries) */", errMySQLMariaDBCode},
	}
	for _, c := range cases {
		if _, err := ClassifyQuery(EngineMySQL, c.sql); !errors.Is(err, c.want) {
			t.Errorf("ClassifyQuery(%q) = %v, want %v", c.sql, err, c.want)
		}
	}
	for _, sql := range []string{
		"SELECT id FROM employees --\ttab comment",
		"SELECT id FROM employees --\r\n",
		"SELECT id FROM employees -- trailing comment",
		"SELECT id FROM employees # hash comment",
		"SELECT id /* inline */ FROM employees",
		"SELECT '--1' FROM employees",
		"SELECT id FROM employees WHERE name = 'a//b'",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

func TestMySQLOpaqueFormsRefused(t *testing.T) {
	for _, sql := range []string{
		"SHOW COLUMNS FROM employees WHERE (SELECT COUNT(*) FROM salaries) > 0",
		"SHOW STATUS WHERE SLEEP(1)",
		"SHOW VARIABLES WHERE Variable_name = 'x'",
		"SHOW INDEX FROM employees WHERE Key_name = 'PRIMARY'",
		"SHOW DATABASES WHERE `Database` IN (SELECT secret FROM salaries)",
		"DESCRIBE employees (SELECT 1)",
		"EXPLAIN WITH x AS (SELECT 1) SELECT * FROM x",
		"EXPLAIN ANALYZE WITH x AS (SELECT secret FROM salaries) SELECT SLEEP(1) FROM x",
		"EXPLAIN (SELECT * FROM employees)",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); !errors.Is(err, errMySQLOpaque) {
			t.Errorf("ClassifyQuery(%q) = %v, want %v", sql, err, errMySQLOpaque)
		}
	}
	for _, sql := range []string{
		"SHOW TABLES",
		"SHOW TABLES LIKE 'emp%'",
		"SHOW STATUS LIKE 'Threads%'",
		"SHOW PROCESSLIST",
		"SHOW GRANTS FOR CURRENT_USER()",
		"SHOW CREATE TABLE employees",
		"SHOW COLUMNS FROM employees",
		"DESCRIBE employees",
		"EXPLAIN employees",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

func TestMySQLShowTablesFilterIsChecked(t *testing.T) {
	stmt, err := ClassifyQuery(EngineMySQL, "SHOW TABLES WHERE Tables_in_app IN (SELECT name FROM salaries)")
	if err != nil {
		t.Fatalf("ClassifyQuery: %v", err)
	}
	if !stmt.ListsTables || len(stmt.Tables) != 1 || stmt.Tables[0] != "salaries" {
		t.Errorf("Statement = %+v, want ListsTables with the subquery's table salaries", stmt)
	}
}

func TestMySQLReadLocksRefused(t *testing.T) {
	for _, sql := range []string{
		"SELECT id FROM employees FOR UPDATE",
		"SELECT id FROM employees LOCK IN SHARE MODE",
		"SELECT id FROM employees WHERE id IN (SELECT id FROM employees FOR UPDATE)",
		"SELECT id FROM employees UNION SELECT id FROM employees FOR UPDATE",
		"EXPLAIN ANALYZE SELECT id FROM employees FOR UPDATE",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); !errors.Is(err, errMySQLReadLocks) {
			t.Errorf("ClassifyQuery(%q) = %v, want %v", sql, err, errMySQLReadLocks)
		}
	}
	for _, sql := range []string{
		"SELECT id FROM employees FOR SHARE",
		"SELECT id FROM employees FOR UPDATE NOWAIT",
		"SELECT id FROM employees FOR UPDATE SKIP LOCKED",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); err == nil {
			t.Errorf("ClassifyQuery(%q): expected a refusal, got nil", sql)
		}
	}
}

// TestMySQLUnparsedFormsRefused pins forms that write or set state which the parser can't read, so they never reach the database.
func TestMySQLUnparsedFormsRefused(t *testing.T) {
	for _, sql := range []string{
		"SELECT @x := 1",
		"SET @x = 1",
		"SELECT id INTO @v FROM employees",
		"SELECT * FROM employees INTO OUTFILE '/tmp/out'",
		"SELECT * FROM employees INTO DUMPFILE '/tmp/out'",
		"WITH d AS (SELECT 1) SELECT * FROM d",
		"SELECT ROW_NUMBER() OVER (ORDER BY id) FROM employees",
		"SELECT * FROM JSON_TABLE('[]', '$[*]' COLUMNS(x INT PATH '$')) j",
		"LOAD DATA LOCAL INFILE '/etc/passwd' INTO TABLE employees",
		"DO SLEEP(1)",
		"CALL leak()",
		"HANDLER employees OPEN",
		"TABLE salaries",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); err == nil {
			t.Errorf("ClassifyQuery(%q): expected a refusal, got nil", sql)
		}
	}
}

// TestMySQLLexerAgreesWithServer pins the comment forms where the parser and MySQL agree: "#" and "--" plus whitespace hide the rest of the line from both, and /*!NNNNN */ content is code to the parser whatever its version, so it sees at least what the server runs.
func TestMySQLLexerAgreesWithServer(t *testing.T) {
	stmt := classifyFor(t, EngineMySQL, "SELECT id FROM employees # , (SELECT secret FROM salaries)\n")
	if len(stmt.Tables) != 1 || stmt.Tables[0] != "employees" {
		t.Errorf("# comment: Tables = %v, want [employees]", stmt.Tables)
	}
	stmt = classifyFor(t, EngineMySQL, "SELECT id FROM employees --\t, (SELECT secret FROM salaries)\n")
	if len(stmt.Tables) != 1 {
		t.Errorf("-- tab comment: Tables = %v, want [employees]", stmt.Tables)
	}
	for _, sql := range []string{
		"SELECT id /*!80000 , (SELECT secret FROM salaries) */ FROM employees",
		"SELECT id /*!99999 , (SELECT secret FROM salaries) */ FROM employees",
		"SELECT id /*! , (SELECT secret FROM salaries) */ FROM employees",
	} {
		stmt := classifyFor(t, EngineMySQL, sql)
		if !reflect.DeepEqual(stmt.Tables, []string{"employees", "salaries"}) {
			t.Errorf("ClassifyQuery(%q).Tables = %v, want the commented table seen too", sql, stmt.Tables)
		}
	}
	if _, err := ClassifyQuery(EngineMySQL, "SELECT 1 /*! 'a*/' , (SELECT secret FROM salaries) , '*/'"); err == nil {
		t.Error("a quote spanning the end of /*! */ parsed, but MySQL would read it as one string")
	}
}

func TestMySQLCatalogNamesCollected(t *testing.T) {
	cases := []struct {
		sql  string
		want []string
	}{
		{"SELECT id FROM employees", nil},
		{"SELECT UPPER(name), COUNT(*), CAST(id AS CHAR), SUBSTRING(name, 1) FROM employees GROUP BY name", []string{"count", "upper"}},
		{"EXPLAIN SELECT format_pico_time(1)", []string{"format_pico_time"}},
		{"SHOW TABLES WHERE Tables_in_app = LOWER('x')", []string{"lower"}},
		{"INSERT INTO employees (name) VALUES (CONCAT('a', 'b'))", []string{"concat"}},
		{"UPDATE employees SET name = sum WHERE id = ABS(-1)", []string{"abs"}},
		{"DELETE FROM employees WHERE hired < NOW()", []string{"now"}},
	}
	for _, c := range cases {
		stmt := classifyFor(t, EngineMySQL, c.sql)
		if got := stmt.Catalog.Functions; !(len(got) == 0 && len(c.want) == 0) && !reflect.DeepEqual(got, c.want) {
			t.Errorf("ClassifyQuery(%q).Catalog.Functions = %v, want %v", c.sql, stmt.Catalog.Functions, c.want)
		}
	}
}

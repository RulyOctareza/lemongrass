// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyQueryPostgresAllowsBuiltinCalls(t *testing.T) {
	for _, sql := range []string{
		"SELECT lower(name), pg_catalog.upper(name), count(*), max(id) FROM employees",
		"SELECT extract(year FROM now()), substring(name FROM 1 FOR 2), trim(name), position('a' IN name) FROM employees",
		"SELECT created_at AT TIME ZONE 'UTC', name SIMILAR TO 'a%', name ILIKE 'a%' FROM employees",
		"SELECT id BETWEEN 1 AND 2, id IN (1, 2), id IS DISTINCT FROM 3, coalesce(name, ''), greatest(id, 1), nullif(id, 0) FROM employees",
		"SELECT id::text, CAST(id AS bigint), id::numeric(10, 2), name::varchar(3), id::double precision, ARRAY[1]::int[] FROM employees",
		"SELECT data->>'k', data #> '{a}', data @> '{}'::jsonb, jsonb_build_object('a', id) FROM employees",
		"SELECT row_number() OVER (ORDER BY id), array_agg(id ORDER BY id) FROM employees",
		"SELECT id = ANY (SELECT emp_id FROM salaries), id = ANY (ARRAY[1, 2]) FROM employees",
		"SELECT current_date, current_timestamp, date_trunc('day', now())",
		"SELECT * FROM generate_series(1, 3) AS g(n)",
		"SELECT * FROM ROWS FROM (generate_series(1, 2), unnest(ARRAY['a'])) AS t(a int, b text)",
		"SELECT * FROM employees e, LATERAL jsonb_each(e.data)",
		"SELECT * FROM employees TABLESAMPLE bernoulli (10)",
		"SELECT JSON_VALUE(data, '$.a'), JSON_QUERY(data, '$.b'), JSON_EXISTS(data, '$.c'), JSON_OBJECT('a': id) FROM employees",
		"SELECT pg_size_pretty(total_exec_time::bigint) FROM pg_stat_statements",
		"UPDATE employees SET name = upper(name) WHERE id = 1",
		"INSERT INTO employees (name) VALUES (concat('a', 'b'))",
	} {
		if _, err := ClassifyQuery(EnginePostgres, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

func TestClassifyQueryPostgresRefusesFunctions(t *testing.T) {
	cases := []struct {
		sql  string
		name string
	}{
		{"SELECT query_to_xml('SELECT * FROM salaries', true, false, '')", "query_to_xml"},
		{"SELECT table_to_xml('salaries', true, false, '')", "table_to_xml"},
		{"SELECT cursor_to_xml('c', 1, true, false, '')", "cursor_to_xml"},
		{"SELECT * FROM dblink('dbname=x', 'SELECT 1') AS t(a int)", "dblink"},
		{"SELECT dblink_exec('dbname=x', 'DELETE FROM salaries')", "dblink_exec"},
		{"SELECT lo_export(1, '/tmp/x')", "lo_export"},
		{"SELECT lo_import('/etc/passwd')", "lo_import"},
		{"SELECT pg_read_file('/etc/passwd')", "pg_read_file"},
		{"SELECT pg_read_binary_file('/etc/passwd')", "pg_read_binary_file"},
		{"SELECT pg_ls_dir('.')", "pg_ls_dir"},
		{"SELECT set_config('search_path', 'public', false)", "set_config"},
		{"SELECT current_setting('search_path')", "current_setting"},
		{"SELECT setval('employees_id_seq', 1)", "setval"},
		{"SELECT nextval('employees_id_seq')", "nextval"},
		{"SELECT currval('employees_id_seq')", "currval"},
		{"SELECT pg_advisory_lock(1)", "pg_advisory_lock"},
		{"SELECT pg_advisory_xact_lock(1)", "pg_advisory_xact_lock"},
		{"SELECT pg_try_advisory_lock(1)", "pg_try_advisory_lock"},
		{"SELECT pg_terminate_backend(1)", "pg_terminate_backend"},
		{"SELECT pg_cancel_backend(1)", "pg_cancel_backend"},
		{"SELECT pg_sleep(10)", "pg_sleep"},
		{"SELECT pg_sleep_for('1 minute')", "pg_sleep_for"},
		{"SELECT pg_relation_size('salaries')", "pg_relation_size"},
		{"SELECT has_table_privilege('salaries', 'select')", "has_table_privilege"},
		{"SELECT pg_stat_statements_reset()", "pg_stat_statements_reset"},
		{"SELECT pg_notify('c', 'x')", "pg_notify"},
		{"SELECT random()", "random"},
		{"SELECT leak(id) FROM employees", "leak"},
		{"SELECT * FROM employees WHERE id = leak(1)", "leak"},
		{"SELECT public.lower(name) FROM employees", "public.lower"},
		{"SELECT mydb.pg_catalog.lower(name) FROM employees", "mydb.pg_catalog.lower"},
		{"SELECT * FROM leak_rows()", "leak_rows"},
		{"SELECT * FROM ROWS FROM (generate_series(1, 2), public.leak_rows()) AS t(a int, b text)", "public.leak_rows"},
		{"SELECT count(*) FILTER (WHERE leak(id) > 0) FROM employees", "leak"},
		{"SELECT * FROM employees ORDER BY leak(id)", "leak"},
		{"WITH c AS (SELECT leak(1)) SELECT * FROM c", "leak"},
		{"EXPLAIN ANALYZE SELECT pg_sleep(10)", "pg_sleep"},
		{"UPDATE employees SET name = 'a' WHERE id = leak(1)", "leak"},
		{"INSERT INTO employees (id) VALUES (nextval('salaries_id_seq'))", "nextval"},
		{"DELETE FROM employees WHERE id = 1 RETURNING leak(id)", "leak"},
	}
	for _, c := range cases {
		_, err := ClassifyQuery(EnginePostgres, c.sql)
		var refused *ErrFunctionNotAllowed
		if !errors.As(err, &refused) || refused.Name != c.name {
			t.Errorf("ClassifyQuery(%q) error = %v, want ErrFunctionNotAllowed for %q", c.sql, err, c.name)
		}
	}
}

func TestClassifyQueryPostgresRefusesOperatorsTypesAndSamples(t *testing.T) {
	cases := []struct {
		sql  string
		want string
	}{
		{"SELECT id OPERATOR(public.+) 1 FROM employees", "operator public.+ is not allowed"},
		{"SELECT id <<< 1 FROM employees", "operator <<< is not allowed"},
		{"SELECT id = ANY (SELECT emp_id FROM salaries) FROM employees WHERE id OPERATOR(public.===) ANY (ARRAY[1])", "operator public.=== is not allowed"},
		{"SELECT array_agg(id ORDER BY id USING OPERATOR(public.<<<)) FROM employees", "operator public.<<< is not allowed"},
		{"SELECT id::public.leaky FROM employees", "type public.leaky is not allowed"},
		{"SELECT CAST(id AS leaky) FROM employees", "type leaky is not allowed"},
		{"SELECT 'salaries'::regclass", "type regclass is not allowed"},
		{"SELECT * FROM generate_series(1, 2) AS g(n public.leaky)", "type public.leaky is not allowed"},
		{"SELECT * FROM employees TABLESAMPLE leaky_sample (10)", "tablesample method leaky_sample is not allowed"},
		{"SELECT JSON_VALUE(data, '$.a' RETURNING public.leaky) FROM employees", "type public.leaky is not allowed"},
		{"SELECT JSON_OBJECT('a': 1 RETURNING leaky)", "type leaky is not allowed"},
		{"SELECT JSON_VALUE(data, '$.a' DEFAULT leak(1) ON EMPTY) FROM employees", "function leak is not allowed"},
	}
	for _, c := range cases {
		_, err := ClassifyQuery(EnginePostgres, c.sql)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ClassifyQuery(%q) error = %v, want %q", c.sql, err, c.want)
		}
	}
}

func TestClassifyQueryPostgresOrdinaryStatementsPass(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM employees",
		"SELECT DISTINCT ON (dept) dept, name FROM employees ORDER BY dept, name DESC NULLS LAST",
		"SELECT e.name, s.amount FROM employees e LEFT JOIN salaries s ON s.emp_id = e.id WHERE s.amount IS NOT NULL",
		"SELECT * FROM employees e NATURAL JOIN salaries",
		"SELECT * FROM employees JOIN salaries USING (id)",
		"SELECT * FROM employees CROSS JOIN LATERAL (SELECT max(amount) FROM salaries WHERE emp_id = employees.id) m",
		"SELECT dept, count(*), sum(id) FILTER (WHERE active) FROM employees GROUP BY dept HAVING count(*) > 1",
		"SELECT dept, team, count(*) FROM employees GROUP BY ROLLUP (dept, team)",
		"SELECT dept, count(*), grouping(dept) FROM employees GROUP BY CUBE (dept), GROUPING SETS ((dept), ())",
		"SELECT name, rank() OVER w, sum(id) OVER (PARTITION BY dept ORDER BY id ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) FROM employees WINDOW w AS (ORDER BY id)",
		"SELECT id FROM employees UNION SELECT emp_id FROM salaries EXCEPT SELECT 1 INTERSECT SELECT 2",
		"SELECT * FROM employees WHERE EXISTS (SELECT 1 FROM salaries WHERE emp_id = employees.id) AND NOT active OR id IS NULL",
		"SELECT * FROM employees WHERE active IS TRUE AND name LIKE 'a%' AND name NOT ILIKE 'b%' AND id NOT IN (1, 2)",
		"SELECT CASE WHEN id > 1 THEN 'a' ELSE 'b' END, CASE dept WHEN 'x' THEN 1 END FROM employees",
		"SELECT (ARRAY[1, 2, 3])[1:2], (ARRAY[1, 2])[1], ROW(1, 'a'), name COLLATE \"C\" FROM employees",
		"SELECT 1.5, -2, B'101', X'ff', true, NULL, 'a' || 'b', E'\\n', $$x$$",
		"SELECT current_date, current_timestamp, localtime, current_user, session_user, interval '1 day', date '2026-01-01'",
		"SELECT * FROM employees ORDER BY id LIMIT 10 OFFSET 5",
		"SELECT * FROM employees FETCH FIRST 3 ROWS ONLY",
		"TABLE employees",
		"VALUES (1, 'a'), (2, 'b')",
		"WITH RECURSIVE t(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM t WHERE n < 3) SELECT * FROM t",
		"SELECT * FROM jsonb_to_recordset('[{\"a\":1}]') AS x(a int)",
		"SELECT xmlelement(name a, xmlattributes(1 AS b), 'c'), xmlforest(name), xmlserialize(content '<a/>' AS text) FROM employees",
		"SELECT * FROM XMLTABLE('/r' PASSING '<r/>' COLUMNS c int PATH '@c')",
		"SELECT JSON_OBJECT('a': id), JSON_ARRAY(1, 2), JSON_ARRAYAGG(id), JSON_OBJECTAGG(name: id), data IS JSON OBJECT FROM employees GROUP BY data",
		"SELECT * FROM JSON_TABLE('[{\"a\":1}]'::jsonb, '$[*]' COLUMNS (a int PATH '$.a'))",
		"SELECT query, calls, total_exec_time FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 5",
		"SELECT relname, seq_scan, idx_scan FROM pg_stat_user_tables",
		"SELECT format('%s', name => 'x')",
		"EXPLAIN (ANALYZE true, FORMAT json) SELECT * FROM employees",
		"INSERT INTO employees (id, name) VALUES (1, 'a'), (2, DEFAULT) RETURNING id",
		"INSERT INTO employees (id, name) VALUES (1, 'a') ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name WHERE employees.active",
		"INSERT INTO employees (id) VALUES (1) ON CONFLICT DO NOTHING",
		"INSERT INTO employees DEFAULT VALUES",
		"UPDATE employees SET (name, dept) = ('a', 'b'), active = DEFAULT WHERE id = 1",
		"UPDATE employees SET (name, dept) = (SELECT payee, 'x' FROM salaries WHERE emp_id = 1) WHERE id = 1",
		"UPDATE employees e SET name = s.payee FROM salaries s WHERE e.id = s.emp_id RETURNING e.*",
		"DELETE FROM employees WHERE id IN (SELECT emp_id FROM salaries WHERE amount < 0) RETURNING *",
	} {
		if _, err := ClassifyQuery(EnginePostgres, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

func TestClassifyQueryPostgresRefusesUnknownNodes(t *testing.T) {
	cases := []struct {
		sql  string
		want string
	}{
		{"INSERT INTO employees (id) SELECT emp_id FROM salaries FOR UPDATE", "LockingClause is not supported"},
		{"UPDATE employees SET name = 'a' WHERE CURRENT OF c", "CurrentOfExpr is not supported"},
	}
	for _, c := range cases {
		_, err := ClassifyQuery(EnginePostgres, c.sql)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ClassifyQuery(%q) error = %v, want %q", c.sql, err, c.want)
		}
	}
	tree := map[string]any{"SelectStmt": map[string]any{"targetList": []any{map[string]any{"SomeFutureExpr": map[string]any{}}}}}
	if _, err := checkPostgresCalls(tree); err == nil || err.Error() != "dbgate: SomeFutureExpr is not supported" {
		t.Errorf("checkPostgresCalls(unknown node) error = %v", err)
	}
}

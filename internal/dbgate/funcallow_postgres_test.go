// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

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
	}
	for _, c := range cases {
		_, err := ClassifyQuery(EnginePostgres, c.sql)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ClassifyQuery(%q) error = %v, want %q", c.sql, err, c.want)
		}
	}
}

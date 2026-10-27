// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

// trickPath is the Gate method a trick is sent through.
type trickPath string

const (
	trickRead  trickPath = "read"
	trickWrite trickPath = "write"
)

// trick is one attempt to get past a channel's scope. Every entry must be refused. The names it uses match the fixtures in scripts/dbgate-realdb: employees and salaries are the tables a channel may grant, secrets is never granted, secret_view reads secrets, bump_salaries writes, read_secrets reads secrets, and an archive schema holds its own employees.
type trick struct {
	name   string
	engine Engine
	sql    string
	scope  vault.Scope
	path   trickPath
	// realOnly marks a trick only the catalog check or the database can refuse, because the parser cannot see the user object it reaches. The unit test skips it.
	realOnly bool
	// dbStops marks a read the read-only transaction must refuse on its own, run again through runQuery with the classifier skipped.
	dbStops bool
}

var (
	trickSelect  = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select"}}
	trickExplain = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", "explain"}}
	trickShow    = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", "show"}}
	trickPerf    = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", OperationPerformance}}
	trickWriteEm = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", "insert", "update", "delete"}}
	trickInsert  = vault.Scope{Tables: []string{"employees"}, Operations: []string{"select", "insert"}}
)

func pgRead(name, sql string) trick {
	return trick{name: name, engine: EnginePostgres, sql: sql, scope: trickSelect, path: trickRead}
}

func myRead(name, sql string) trick {
	return trick{name: name, engine: EngineMySQL, sql: sql, scope: trickSelect, path: trickRead}
}

func (t trick) under(s vault.Scope) trick { t.scope = s; return t }
func (t trick) write() trick              { t.path = trickWrite; return t }
func (t trick) real() trick               { t.realOnly = true; return t }
func (t trick) stopped() trick            { t.dbStops = true; return t }

var postgresTricks = []trick{
	// Writes through a read: data-modifying CTEs, SELECT INTO, table-creating forms.
	pgRead("cte delete in select", "WITH d AS (DELETE FROM employees WHERE id = 1 RETURNING *) SELECT * FROM d").stopped(),
	pgRead("cte update in select", "WITH d AS (UPDATE employees SET name = 'x' WHERE id = 1 RETURNING *) SELECT * FROM d").stopped(),
	pgRead("cte insert in select", "WITH d AS (INSERT INTO employees (id, name) VALUES (900, 'x') RETURNING *) SELECT * FROM d").stopped(),
	pgRead("cte delete of ungranted table", "WITH d AS (DELETE FROM secrets WHERE id = 1 RETURNING id) SELECT count(*) FROM d").stopped(),
	pgRead("cte write nested in subquery", "SELECT * FROM employees WHERE id IN (WITH d AS (DELETE FROM salaries WHERE employee_id = 1 RETURNING employee_id) SELECT employee_id FROM d)"),
	pgRead("cte merge in select", "WITH d AS (MERGE INTO employees e USING (SELECT 1 AS id) s ON e.id = s.id WHEN MATCHED THEN DELETE RETURNING e.*) SELECT * FROM d"),
	pgRead("select into", "SELECT * INTO stolen FROM employees").stopped(),
	pgRead("select into temp", "SELECT * INTO TEMP stolen FROM employees").stopped(),
	pgRead("create table as", "CREATE TABLE stolen AS SELECT * FROM employees"),
	pgRead("select for update", "SELECT * FROM employees FOR UPDATE").stopped(),
	pgRead("select for share in subquery", "SELECT * FROM employees WHERE id IN (SELECT id FROM employees FOR SHARE)").stopped(),

	// Writes to an ungranted table, or without the write grant, through the write path.
	pgRead("cte delete under insert", "WITH d AS (DELETE FROM salaries WHERE employee_id = 1 RETURNING employee_id) INSERT INTO employees (id) SELECT employee_id FROM d").write().under(trickWriteEm),
	pgRead("cte delete under update", "WITH d AS (DELETE FROM secrets WHERE id = 1 RETURNING id) UPDATE employees SET name = 'x' WHERE id IN (SELECT id FROM d)").write().under(trickWriteEm),
	pgRead("cte update under delete", "WITH d AS (UPDATE salaries SET amount = 0 WHERE employee_id = 1 RETURNING employee_id) DELETE FROM employees WHERE id IN (SELECT employee_id FROM d)").write().under(trickWriteEm),
	pgRead("cte delete of granted table without delete grant", "WITH d AS (DELETE FROM employees WHERE id = 1 RETURNING id) INSERT INTO employees (id, name) SELECT id + 1000, 'x' FROM d").write().under(trickInsert),
	pgRead("insert select from ungranted", "INSERT INTO employees (id, name) SELECT id, secret FROM secrets").write().under(trickWriteEm),
	pgRead("update from ungranted", "UPDATE employees e SET name = s.secret FROM secrets s WHERE e.id = s.id").write().under(trickWriteEm),
	pgRead("ungranted target aliased as granted", "DELETE FROM secrets AS employees WHERE employees.id = 1").write().under(trickWriteEm),
	pgRead("ungranted insert target aliased as granted", "INSERT INTO secrets AS employees (id, secret) VALUES (900, 'x')").write().under(trickWriteEm),
	pgRead("update target aliased as granted", "UPDATE secrets AS employees SET secret = 'x' WHERE employees.id = 1").write().under(trickWriteEm),
	pgRead("write to other schema same name", "DELETE FROM archive.employees WHERE id = 1").write().under(trickWriteEm),
	pgRead("merge", "MERGE INTO employees e USING secrets s ON e.id = s.id WHEN MATCHED THEN UPDATE SET name = s.secret").write().under(trickWriteEm),
	pgRead("write function in insert values", "INSERT INTO employees (id, name) VALUES (901, bump_salaries()::text)").write().under(trickWriteEm),
	pgRead("read function in update", "UPDATE employees SET name = read_secrets() WHERE id = 1").write().under(trickWriteEm),
	pgRead("insert returning ungranted subquery", "INSERT INTO employees (id, name) VALUES (902, 'x') RETURNING (SELECT secret FROM secrets LIMIT 1)").write().under(trickWriteEm),
	pgRead("insert on conflict reads ungranted", "INSERT INTO employees (id, name) VALUES (1, 'x') ON CONFLICT (id) DO UPDATE SET name = (SELECT secret FROM secrets LIMIT 1)").write().under(trickWriteEm),
	pgRead("cte name hides read on write path", "WITH secrets AS (SELECT 1 AS x) INSERT INTO employees (id, name) SELECT 950, secret FROM public.secrets LIMIT 1").write().under(trickWriteEm),

	// Side effects through functions in a SELECT.
	pgRead("setval", "SELECT setval('invoice_seq', 1000)").stopped(),
	pgRead("nextval", "SELECT nextval('invoice_seq')").stopped(),
	pgRead("currval", "SELECT currval('invoice_seq')"),
	pgRead("terminate backend", "SELECT pg_terminate_backend(pid) FROM pg_stat_activity"),
	pgRead("cancel backend", "SELECT pg_cancel_backend(1)"),
	pgRead("advisory lock", "SELECT pg_advisory_lock(4242)"),
	pgRead("try advisory lock", "SELECT pg_try_advisory_lock(4242)"),
	pgRead("advisory xact lock", "SELECT pg_advisory_xact_lock(4242)"),
	pgRead("dblink exec", "SELECT dblink_exec('dbname=vaultapp', 'DELETE FROM employees')"),
	pgRead("lo export", "SELECT lo_export(1, '/tmp/x')"),
	pgRead("lo import", "SELECT lo_import('/etc/hostname')"),
	pgRead("sleep", "SELECT pg_sleep(1)"),
	pgRead("sleep for", "SELECT pg_sleep_for('1 second')"),
	pgRead("set_config session", "SELECT set_config('search_path', 'archive', false)"),
	pgRead("notify", "SELECT pg_notify('x', 'y')"),
	pgRead("stat reset", "SELECT pg_stat_statements_reset()"),
	pgRead("write function", "SELECT bump_salaries()").stopped(),
	pgRead("write function in where", "SELECT * FROM employees WHERE bump_salaries() > 0").stopped(),
	pgRead("write function in order by", "SELECT * FROM employees ORDER BY bump_salaries()").stopped(),
	pgRead("write function in from", "SELECT * FROM bump_salaries()").stopped(),
	pgRead("write function in rows from", "SELECT * FROM ROWS FROM (bump_salaries())").stopped(),
	pgRead("write function in lateral", "SELECT * FROM employees e, LATERAL bump_salaries() b").stopped(),
	pgRead("write function in aggregate filter", "SELECT count(*) FILTER (WHERE bump_salaries() > 0) FROM employees").stopped(),
	pgRead("write function in values", "VALUES (bump_salaries())").stopped(),

	// Reads of ungranted tables through functions and views.
	pgRead("query_to_xml", "SELECT query_to_xml('SELECT * FROM secrets', true, false, '')"),
	pgRead("table_to_xml", "SELECT table_to_xml('secrets', true, false, '')"),
	pgRead("cursor_to_xml", "SELECT cursor_to_xml('c', 10, true, false, '')"),
	pgRead("dblink read", "SELECT * FROM dblink('dbname=vaultapp', 'SELECT secret FROM secrets') AS t(secret text)"),
	pgRead("read file", "SELECT pg_read_file('/etc/hostname')"),
	pgRead("read binary file", "SELECT pg_read_binary_file('/etc/hostname')"),
	pgRead("ls dir", "SELECT pg_ls_dir('.')"),
	pgRead("current_setting", "SELECT current_setting('data_directory')"),
	pgRead("relation size of ungranted", "SELECT pg_relation_size('secrets')"),
	pgRead("stat function on ungranted", "SELECT pg_stat_get_live_tuples(16384)"),
	pgRead("read function", "SELECT read_secrets()"),
	pgRead("read function in from", "SELECT * FROM read_secrets()"),
	pgRead("public-qualified read function", "SELECT public.read_secrets()"),
	pgRead("quoted read function", `SELECT "read_secrets"()`),
	pgRead("upper-case read function", "SELECT READ_SECRETS()"),
	pgRead("public-qualified builtin name", "SELECT public.lower('x')"),
	pgRead("quoted upper builtin name", `SELECT "LOWER"('x')`),
	pgRead("pg_catalog sleep", "SELECT pg_catalog.pg_sleep(1)"),
	pgRead("upper-case sleep", "SELECT PG_SLEEP(1)"),
	pgRead("view over ungranted", "SELECT * FROM secret_view"),
	pgRead("view in subquery", "SELECT * FROM employees WHERE id IN (SELECT id FROM secret_view)"),
	pgRead("ungranted in lateral", "SELECT * FROM employees, LATERAL (SELECT secret FROM secrets) s"),
	pgRead("ungranted in scalar subquery", "SELECT (SELECT secret FROM secrets LIMIT 1) FROM employees"),
	pgRead("ungranted in exists", "SELECT * FROM employees WHERE EXISTS (SELECT 1 FROM secrets WHERE secret LIKE 'a%')"),
	pgRead("ungranted in union", "SELECT name FROM employees UNION SELECT secret FROM secrets"),
	pgRead("ungranted in cte", "WITH s AS (SELECT secret FROM secrets) SELECT * FROM s"),
	pgRead("cte named like granted", "WITH employees AS (SELECT secret FROM secrets) SELECT * FROM employees"),
	pgRead("table statement", "TABLE secrets"),
	pgRead("values with subquery", "VALUES ((SELECT secret FROM secrets LIMIT 1))"),
	pgRead("other schema same name", "SELECT * FROM archive.employees"),
	pgRead("only ungranted", "SELECT * FROM ONLY secrets"),

	// CTE-name shadowing: postgresTables collects every CTE name into one global set and drops any RangeVar with that bare relname, ignoring query scope and schema qualification, so a CTE named like an ungranted table hides a real read of it.
	pgRead("cte name hides schema-qualified read", "WITH secrets AS (SELECT 1 AS x) SELECT * FROM public.secrets"),
	pgRead("cte name hides outer read in nested scope", "SELECT * FROM secrets WHERE id IN (WITH secrets AS (SELECT 1 AS id) SELECT id FROM secrets)"),
	pgRead("cte self-reference reads real table", "WITH secrets AS (SELECT * FROM secrets) SELECT * FROM secrets"),

	// Casts, operators and types that reach user or catalog code.
	pgRead("regclass cast", "SELECT 'secrets'::regclass"),
	pgRead("regclass cast of column", "SELECT CAST(name AS regclass) FROM employees"),
	pgRead("regproc cast", "SELECT 'read_secrets'::regproc"),
	pgRead("regnamespace cast", "SELECT 'archive'::regnamespace"),
	pgRead("user type cast", "SELECT 'x'::public.secret_type"),
	pgRead("user operator", "SELECT 1 OPERATOR(public.===) 1"),
	pgRead("unknown operator symbol", "SELECT 1 === 1"),
	pgRead("tablesample user method", "SELECT * FROM employees TABLESAMPLE evil_method (10)"),
	pgRead("attribute notation", "SELECT e.leak FROM employees e").real(),
	pgRead("functional notation of attribute", "SELECT leak(e) FROM employees e"),
	pgRead("builtin overload on int", "SELECT lower(id) FROM employees").real(),
	pgRead("user equality operator", "SELECT * FROM employees WHERE name = 1").real(),
	pgRead("pg_catalog lower cannot reach the int overload", "SELECT pg_catalog.lower(id) FROM employees").real(),
	pgRead("pg_catalog equality cannot reach the user operator", "SELECT * FROM employees WHERE name OPERATOR(pg_catalog.=) 1").real(),

	// SQL/JSON node forms (PG16/17): the node-kind allowlist accepts them as pure syntax but still walks their arguments and checks a RETURNING type, so a user object reached through one is refused.
	pgRead("json_value returning user type", "SELECT JSON_VALUE('{\"a\":1}'::jsonb, '$.a' RETURNING public.secret_type)"),
	pgRead("json_value argument calls read function", "SELECT JSON_VALUE(to_jsonb(read_secrets()), '$')"),
	pgRead("json_value argument reads ungranted", "SELECT JSON_VALUE((SELECT to_jsonb(secret) FROM secrets LIMIT 1), '$')"),

	// Quoting, case and unicode.
	pgRead("quoted ungranted", `SELECT * FROM "secrets"`),
	pgRead("upper-case ungranted", "SELECT * FROM SECRETS"),
	pgRead("unicode escape identifier", `SELECT * FROM U&"s\0065crets"`),
	pgRead("unicode escape with uescape", `SELECT * FROM U&"!0073ecrets" UESCAPE '!'`),
	pgRead("dollar-quoted function body", "SELECT query_to_xml($q$SELECT * FROM secrets$q$, true, false, '')"),

	// Stacked statements, hidden in strings and comments.
	pgRead("stacked delete", "SELECT 1; DELETE FROM employees WHERE id = 1"),
	pgRead("stacked after string", "SELECT ';'; DELETE FROM employees WHERE id = 1"),
	pgRead("stacked after dollar string", "SELECT $$;$$; DELETE FROM employees WHERE id = 1"),
	pgRead("stacked after escape string", `SELECT E'\';'; DELETE FROM employees WHERE id = 1`),
	pgRead("stacked after line comment", "SELECT 1 -- x\n; DELETE FROM employees WHERE id = 1"),
	pgRead("stacked after block comment", "SELECT 1 /* x */; DELETE FROM employees WHERE id = 1"),
	pgRead("stacked after nested comment", "SELECT 1 /* /* */ */; DELETE FROM employees WHERE id = 1"),
	pgRead("stacked after nul byte", "SELECT 1\x00; DELETE FROM employees WHERE id = 1"),

	// Statements no read grant covers.
	pgRead("copy out", "COPY (SELECT * FROM secrets) TO STDOUT"),
	pgRead("copy to program", "COPY employees TO PROGRAM 'true'"),
	pgRead("do block", "DO $$ BEGIN DELETE FROM employees; END $$"),
	pgRead("call", "CALL bump_proc()"),
	pgRead("set", "SET search_path = archive"),
	pgRead("set role", "SET ROLE postgres"),
	pgRead("reset", "RESET ALL"),
	pgRead("lock table", "LOCK TABLE employees"),
	pgRead("declare cursor", "DECLARE c CURSOR FOR SELECT * FROM secrets"),
	pgRead("prepare", "PREPARE p AS SELECT * FROM secrets"),
	pgRead("execute", "EXECUTE p"),
	pgRead("listen", "LISTEN x"),
	pgRead("notify statement", "NOTIFY x"),
	pgRead("discard", "DISCARD ALL"),
	pgRead("truncate", "TRUNCATE employees").write().under(trickWriteEm),
	pgRead("create function", "CREATE FUNCTION f() RETURNS int AS 'SELECT 1' LANGUAGE sql"),
	pgRead("alter table", "ALTER TABLE employees ADD COLUMN x int").write().under(trickWriteEm),
	pgRead("vacuum", "VACUUM employees"),
	pgRead("analyze", "ANALYZE employees"),
	pgRead("refresh", "REFRESH MATERIALIZED VIEW secret_view"),

	// Performance-view abuse.
	pgRead("perf catalog table", "SELECT relname FROM pg_catalog.pg_class").under(trickPerf),
	pgRead("perf authid", "SELECT rolname, rolpassword FROM pg_catalog.pg_authid").under(trickPerf),
	pgRead("perf stat activity", "SELECT query FROM pg_stat_activity").under(trickPerf),
	pgRead("perf view joined with ungranted", "SELECT * FROM pg_locks l, secrets s").under(trickPerf),
	pgRead("perf view with read function", "SELECT read_secrets() FROM pg_stat_user_tables").under(trickPerf),
	pgRead("perf view in other schema", "SELECT * FROM public.pg_locks").under(trickPerf),
	pgRead("perf view without grant", "SELECT * FROM pg_stat_statements"),
	pgRead("information schema", "SELECT * FROM information_schema.tables").under(trickPerf),

	// EXPLAIN routes.
	pgRead("explain analyze delete", "EXPLAIN ANALYZE DELETE FROM employees WHERE id = 1").under(trickExplain),
	pgRead("explain analyze cte delete", "EXPLAIN ANALYZE WITH d AS (DELETE FROM employees WHERE id = 1 RETURNING *) SELECT * FROM d").under(trickExplain),
	pgRead("explain analyze select into", "EXPLAIN ANALYZE SELECT * INTO stolen FROM employees").under(trickExplain),
	pgRead("explain analyze create table as", "EXPLAIN ANALYZE CREATE TABLE stolen AS SELECT 1").under(trickExplain),
	pgRead("explain analyze ungranted", "EXPLAIN ANALYZE SELECT * FROM secrets").under(trickExplain),
	pgRead("explain analyze write function", "EXPLAIN ANALYZE SELECT bump_salaries()").under(trickExplain),
	pgRead("explain analyze sleep", "EXPLAIN ANALYZE SELECT pg_sleep(1)").under(trickExplain),
	pgRead("explain analyze nextval", "EXPLAIN ANALYZE SELECT nextval('invoice_seq')").under(trickExplain),
	pgRead("explain execute", "EXPLAIN EXECUTE p").under(trickExplain),
	pgRead("explain without grant", "EXPLAIN SELECT * FROM employees"),
}

var mysqlTricks = []trick{
	// Side effects through functions.
	myRead("sleep", "SELECT SLEEP(1)"),
	myRead("benchmark", "SELECT BENCHMARK(1000, MD5('x'))"),
	myRead("get_lock", "SELECT GET_LOCK('vault_trick', 0)"),
	myRead("release_all_locks", "SELECT RELEASE_ALL_LOCKS()"),
	myRead("is_used_lock", "SELECT IS_USED_LOCK('vault_trick')"),
	myRead("load_file", "SELECT LOAD_FILE('/etc/hostname')"),
	myRead("last_insert_id with argument", "SELECT LAST_INSERT_ID(5)"),
	myRead("uuid_short", "SELECT UUID_SHORT()"),
	myRead("sleep in where", "SELECT * FROM employees WHERE SLEEP(1) = 0"),
	myRead("sleep in group_concat", "SELECT GROUP_CONCAT(SLEEP(1)) FROM employees"),
	myRead("sleep in cast", "SELECT CAST(SLEEP(1) AS CHAR)"),
	myRead("sleep in case", "SELECT CASE WHEN SLEEP(1) THEN 1 END"),
	myRead("sleep in derived table", "SELECT * FROM (SELECT SLEEP(1) AS s) t"),
	myRead("sleep in order by", "SELECT * FROM employees ORDER BY SLEEP(1)"),
	myRead("write function", "SELECT bump_salaries()").stopped(),
	myRead("write function in where", "SELECT * FROM employees WHERE bump_salaries() > 0").stopped(),
	myRead("next invoice", "SELECT next_invoice()").stopped(),
	myRead("read function", "SELECT read_secrets()"),
	myRead("qualified read function", "SELECT vaultapp.read_secrets()"),
	myRead("backticked read function", "SELECT `read_secrets`()"),
	myRead("backticked qualified builtin name", "SELECT `vaultapp`.`concat`('a')"),
	myRead("upper-case read function", "SELECT READ_SECRETS()"),
	myRead("sys stored function", "SELECT sys.format_time(1)"),
	myRead("space before builtin", "SELECT sum (id) FROM employees"),
	myRead("comment before builtin", "SELECT sum/**/(id) FROM employees"),
	myRead("stored sum shadowing builtin", "SELECT sum(id) FROM employees").real(),
	myRead("stored format_pico_time", "SELECT format_pico_time(id) FROM employees").real(),
	myRead("extractvalue", "SELECT EXTRACTVALUE('<a/>', '/a')"),
	myRead("statement_digest", "SELECT STATEMENT_DIGEST('SELECT 1')"),

	// Writes and session state through a read.
	myRead("select into outfile", "SELECT * FROM employees INTO OUTFILE '/tmp/x'"),
	myRead("select into dumpfile", "SELECT * FROM employees INTO DUMPFILE '/tmp/x'"),
	myRead("select into variable", "SELECT id INTO @x FROM employees LIMIT 1"),
	myRead("variable assignment", "SELECT @x := 1"),
	myRead("variable assignment in where", "SELECT * FROM employees WHERE (@x := id) > 0"),
	myRead("select for update", "SELECT * FROM employees FOR UPDATE"),
	myRead("select for share", "SELECT * FROM employees FOR SHARE"),
	myRead("lock in share mode", "SELECT * FROM employees LOCK IN SHARE MODE"),
	myRead("for update in union", "SELECT id FROM employees UNION SELECT id FROM employees FOR UPDATE"),
	// Union.walkSubtree skips the UNION's own ORDER BY and LIMIT, so a subquery or call there escaped every MySQL walk and reached the server.
	myRead("union order by reads ungranted", "SELECT id FROM employees UNION SELECT id FROM employees ORDER BY (SELECT secret FROM secrets LIMIT 1)"),
	myRead("union order by sleep", "SELECT id FROM employees UNION SELECT id FROM employees ORDER BY SLEEP(1)"),
	myRead("cte", "WITH s AS (SELECT secret FROM secrets) SELECT * FROM s"),
	myRead("recursive cte", "WITH RECURSIVE r AS (SELECT 1 AS n UNION ALL SELECT n + 1 FROM r) SELECT * FROM r"),
	myRead("json_table", "SELECT * FROM JSON_TABLE('[1]', '$[*]' COLUMNS (x INT PATH '$')) t"),
	myRead("lateral", "SELECT * FROM employees e, LATERAL (SELECT secret FROM secrets) s"),

	// Reads of ungranted tables.
	myRead("view over ungranted", "SELECT * FROM secret_view"),
	myRead("view in subquery", "SELECT * FROM employees WHERE id IN (SELECT id FROM secret_view)"),
	myRead("ungranted in scalar subquery", "SELECT (SELECT secret FROM secrets LIMIT 1) FROM employees"),
	myRead("ungranted in union", "SELECT name FROM employees UNION SELECT secret FROM secrets"),
	myRead("ungranted in exists", "SELECT * FROM employees WHERE EXISTS (SELECT 1 FROM secrets)"),
	myRead("other database same name", "SELECT * FROM vault_archive.employees"),
	myRead("backticked other database", "SELECT * FROM `vault_archive`.`employees`"),
	myRead("backticked ungranted", "SELECT * FROM `secrets`"),
	myRead("ungranted with alias named granted", "SELECT * FROM secrets AS employees"),
	myRead("table statement", "TABLE secrets"),
	myRead("values statement", "VALUES ROW((SELECT secret FROM secrets LIMIT 1))"),
	myRead("handler", "HANDLER secrets OPEN"),

	// Comments the parser and MySQL may read differently.
	myRead("double dash without space", "SELECT 1 --1, (SELECT secret FROM secrets)"),
	myRead("double dash without space before newline", "SELECT 1 --1\n, (SELECT secret FROM secrets LIMIT 1)"),
	myRead("versioned comment", "SELECT id /*!, (SELECT secret FROM secrets LIMIT 1) */ FROM employees"),
	myRead("versioned comment with version", "SELECT id /*!50000 , (SELECT secret FROM secrets LIMIT 1) */ FROM employees"),
	myRead("versioned comment hides sleep", "SELECT 1 /*!50000 + SLEEP(1) */"),
	myRead("mariadb executable comment", "SELECT 1 /*M! , (SELECT secret FROM secrets LIMIT 1) */"),
	myRead("mariadb versioned executable comment", "SELECT 1 /*M!100100 , (SELECT secret FROM secrets LIMIT 1) */"),
	myRead("optimizer hint sets sql_mode", "SELECT /*+ SET_VAR(sql_mode='ANSI_QUOTES') */ \"query_sample_text\" FROM performance_schema.events_statements_summary_by_digest").under(trickPerf),
	myRead("hash comment before ungranted", "SELECT id # x\n, (SELECT secret FROM secrets LIMIT 1) FROM employees"),

	// Stacked statements.
	myRead("stacked delete", "SELECT 1; DELETE FROM employees WHERE id = 1"),
	myRead("stacked after string", "SELECT ';'; DELETE FROM employees WHERE id = 1"),
	myRead("stacked after escaped quote", `SELECT '\';'; DELETE FROM employees WHERE id = 1`),
	myRead("stacked after backticks", "SELECT 1 AS `;`; DELETE FROM employees WHERE id = 1"),
	myRead("stacked after block comment", "SELECT 1 /* ; */; DELETE FROM employees WHERE id = 1"),
	myRead("stacked after versioned comment", "SELECT 1 /*!; DELETE FROM employees WHERE id = 1 */"),
	myRead("stacked after line comment", "SELECT 1 -- x\n; DELETE FROM employees WHERE id = 1"),
	myRead("stacked after nul byte", "SELECT 1\x00; DELETE FROM employees WHERE id = 1"),

	// Statements no read grant covers.
	myRead("do sleep", "DO SLEEP(1)"),
	myRead("set variable", "SET @x = 1"),
	myRead("set sql_mode", "SET sql_mode = 'ANSI_QUOTES'"),
	myRead("call", "CALL bump_proc()"),
	myRead("lock tables", "LOCK TABLES employees WRITE"),
	myRead("load data", "LOAD DATA LOCAL INFILE '/etc/hostname' INTO TABLE employees"),
	myRead("prepare", "PREPARE p FROM 'DELETE FROM employees'"),
	myRead("execute", "EXECUTE p"),
	myRead("flush", "FLUSH TABLES"),
	myRead("kill", "KILL 1"),
	myRead("truncate", "TRUNCATE TABLE employees").write().under(trickWriteEm),
	myRead("create table select", "CREATE TABLE stolen SELECT * FROM employees"),
	myRead("rename table", "RENAME TABLE secrets TO employees_old"),

	// SHOW and DESCRIBE.
	myRead("show columns with subquery", "SHOW COLUMNS FROM employees WHERE (SELECT COUNT(*) FROM secrets) > 0").under(trickShow),
	myRead("show status with sleep", "SHOW STATUS WHERE SLEEP(1)").under(trickShow),
	myRead("show tables with subquery", "SHOW TABLES WHERE (SELECT COUNT(*) FROM secrets) > 0").under(trickShow),
	myRead("show create table ungranted", "SHOW CREATE TABLE secrets").under(trickShow),
	myRead("show index ungranted", "SHOW INDEX FROM secrets").under(trickShow),
	myRead("describe ungranted", "DESCRIBE secrets").under(trickShow),
	myRead("describe other database", "DESCRIBE vault_archive.employees").under(trickShow),
	myRead("show without grant", "SHOW TABLES"),

	// Performance-view abuse.
	myRead("perf star with restricted column", "SELECT * FROM performance_schema.events_statements_summary_by_digest").under(trickPerf),
	myRead("perf restricted column", "SELECT query_sample_text FROM performance_schema.events_statements_summary_by_digest").under(trickPerf),
	myRead("perf restricted column through alias", "SELECT d.query_sample_text FROM performance_schema.events_statements_summary_by_digest d").under(trickPerf),
	myRead("perf restricted column in subquery", "SELECT COUNT(*) FROM employees WHERE 'x' IN (SELECT query_sample_text FROM performance_schema.events_statements_summary_by_digest)").under(trickPerf),
	myRead("perf restricted column through derived star", "SELECT t.digest FROM (SELECT * FROM performance_schema.events_statements_summary_by_digest) t").under(trickPerf),
	myRead("perf restricted column backticked upper", "SELECT `QUERY_SAMPLE_TEXT` FROM performance_schema.events_statements_summary_by_digest").under(trickPerf),
	myRead("perf lock_data", "SELECT lock_data FROM performance_schema.data_locks").under(trickPerf),
	myRead("perf statement history", "SELECT sql_text FROM performance_schema.events_statements_history").under(trickPerf),
	myRead("perf threads", "SELECT * FROM performance_schema.threads").under(trickPerf),
	myRead("perf processlist", "SELECT info FROM information_schema.processlist").under(trickPerf),
	myRead("perf mysql user", "SELECT authentication_string FROM mysql.user").under(trickPerf),
	myRead("perf view joined with ungranted", "SELECT * FROM sys.host_summary, secrets").under(trickPerf),
	myRead("perf view with read function", "SELECT read_secrets() FROM sys.host_summary").under(trickPerf),
	myRead("perf schema name as user database", "SELECT * FROM vaultapp.events_statements_summary_by_digest").under(trickPerf),
	myRead("perf view without grant", "SELECT digest_text FROM performance_schema.events_statements_summary_by_digest"),

	// Writes through the write path.
	myRead("insert select from ungranted", "INSERT INTO employees (id, name) SELECT id, secret FROM secrets").write().under(trickWriteEm),
	myRead("insert into ungranted", "INSERT INTO secrets (id, secret) VALUES (900, 'x')").write().under(trickWriteEm),
	myRead("replace into ungranted", "REPLACE INTO secrets (id, secret) VALUES (900, 'x')").write().under(trickWriteEm),
	myRead("insert into other database", "INSERT INTO vault_archive.employees (id, name) VALUES (900, 'x')").write().under(trickWriteEm),
	myRead("multi-table update writes ungranted", "UPDATE employees e, secrets s SET s.secret = e.name WHERE e.id = s.id").write().under(trickWriteEm),
	myRead("multi-table update unqualified set", "UPDATE employees e JOIN salaries s ON e.id = s.employee_id SET amount = 0 WHERE e.id = 1").write().under(trickWriteEm),
	myRead("multi-table delete through alias", "DELETE s FROM employees e JOIN secrets s ON e.id = s.id WHERE e.id = 1").write().under(trickWriteEm),
	myRead("delete target aliased as granted", "DELETE employees FROM secrets AS employees WHERE employees.id = 1").write().under(trickWriteEm),
	myRead("update target aliased as granted", "UPDATE secrets AS employees SET secret = 'x' WHERE employees.id = 1").write().under(trickWriteEm),
	myRead("delete without delete grant", "DELETE e FROM employees e WHERE e.id = 1").write().under(trickInsert),
	myRead("write function in insert values", "INSERT INTO employees (id, name) VALUES (901, bump_salaries())").write().under(trickWriteEm),
	myRead("read function in update", "UPDATE employees SET name = read_secrets() WHERE id = 1").write().under(trickWriteEm),
	myRead("sleep in delete where", "DELETE FROM employees WHERE id = 1 AND SLEEP(1) = 0").write().under(trickWriteEm),
	myRead("on duplicate reads ungranted", "INSERT INTO employees (id, name) VALUES (1, 'x') ON DUPLICATE KEY UPDATE name = (SELECT secret FROM secrets LIMIT 1)").write().under(trickWriteEm),

	// EXPLAIN routes.
	myRead("explain analyze ungranted", "EXPLAIN ANALYZE SELECT * FROM secrets").under(trickExplain),
	myRead("explain analyze sleep", "EXPLAIN ANALYZE SELECT SLEEP(1)").under(trickExplain),
	myRead("explain analyze write function", "EXPLAIN ANALYZE SELECT bump_salaries()").under(trickExplain),
	myRead("explain delete", "EXPLAIN DELETE FROM employees WHERE id = 1").under(trickExplain),
	myRead("explain analyze cte", "EXPLAIN ANALYZE WITH x AS (SELECT * FROM secrets) SELECT * FROM x").under(trickExplain),
	myRead("explain format json ungranted", "EXPLAIN FORMAT=JSON SELECT * FROM secrets").under(trickExplain),
	myRead("describe select ungranted", "DESCRIBE SELECT * FROM secrets").under(trickExplain),
	myRead("explain constant subquery", "EXPLAIN SELECT (SELECT secret FROM secrets LIMIT 1) FROM employees").under(trickExplain),
	myRead("explain for connection", "EXPLAIN FOR CONNECTION 1").under(trickExplain),
	myRead("explain without grant", "EXPLAIN SELECT * FROM employees"),
}

func allTricks() []trick {
	return append(append([]trick{}, postgresTricks...), mysqlTricks...)
}

// refuseTrick mirrors the order Gate.Query and Gate.Execute check in, without a server: classification, then the path the statement's kind belongs to, then the channel's scope. A nil return means the trick got through every check the classifier owns.
func refuseTrick(tr trick) error {
	stmt, err := ClassifyQuery(tr.engine, tr.sql)
	if err != nil {
		return err
	}
	if isWriteKind(stmt.Kind) != (tr.path == trickWrite) {
		return errWrongPath
	}
	return AllowStatement(tr.scope, stmt)
}

var errWrongPath = &ErrScopeViolation{Operation: "this statement on this path"}

func TestTricksRefusedByClassifier(t *testing.T) {
	for _, tr := range allTricks() {
		if tr.realOnly {
			continue
		}
		t.Run(string(tr.engine)+"/"+tr.name, func(t *testing.T) {
			if err := refuseTrick(tr); err == nil {
				t.Errorf("%s %s trick was allowed: %q", tr.engine, tr.path, tr.sql)
			}
		})
	}
}

func TestTrickNamesAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, tr := range allTricks() {
		key := string(tr.engine) + "/" + tr.name
		if seen[key] {
			t.Errorf("duplicate trick name %s", key)
		}
		seen[key] = true
	}
}

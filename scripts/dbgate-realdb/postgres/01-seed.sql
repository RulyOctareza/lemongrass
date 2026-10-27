-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (C) 2026 The Lemongrass Authors

-- Fake fixtures for internal/dbgate/tricks_realdb_test.go, run as the vaultadmin superuser by the image on a fresh volume, inside vaultapp.
-- A channel may grant employees and salaries; nothing ever grants secrets. Every name here is used by tricks_test.go.

CREATE EXTENSION pg_stat_statements;
CREATE EXTENSION dblink;
CREATE EXTENSION citext;

CREATE TABLE employees (id int PRIMARY KEY, name text NOT NULL DEFAULT '');
CREATE TABLE salaries (employee_id int PRIMARY KEY, amount int NOT NULL);
CREATE TABLE secrets (id int PRIMARY KEY, secret text NOT NULL);
CREATE SEQUENCE invoice_seq START 100;

INSERT INTO employees VALUES (1, 'Ada Example'), (2, 'Bo Sample'), (3, 'Cy Placeholder');
INSERT INTO salaries VALUES (1, 5000), (2, 6000), (3, 7000);
INSERT INTO secrets VALUES (1, 'fake-secret-alpha'), (2, 'fake-secret-beta');

CREATE VIEW secret_view AS SELECT id, secret FROM secrets;
CREATE DOMAIN secret_type AS text;

CREATE FUNCTION read_secrets() RETURNS text LANGUAGE sql
  AS $$ SELECT string_agg(secret, ',' ORDER BY id) FROM secrets $$;

CREATE FUNCTION bump_salaries() RETURNS int LANGUAGE plpgsql
  AS $$ BEGIN UPDATE salaries SET amount = amount + 1; RETURN 1; END $$;

CREATE PROCEDURE bump_proc() LANGUAGE sql
  AS $$ UPDATE salaries SET amount = amount + 1 $$;

-- Code the parser cannot see without a catalog: an int overload of an allowlisted name, a one-argument function reached by attribute notation (e.leak), and a user = operator on (text, int4).
CREATE FUNCTION public.lower(int4) RETURNS text LANGUAGE sql
  AS $$ SELECT string_agg(secret, ',' ORDER BY id) FROM secrets $$;

CREATE FUNCTION leak(employees) RETURNS text LANGUAGE sql
  AS $$ SELECT string_agg(secret, ',' ORDER BY id) FROM secrets $$;

CREATE FUNCTION text_eq_int(text, int4) RETURNS boolean LANGUAGE sql
  AS $$ SELECT EXISTS (SELECT 1 FROM secrets WHERE secret LIKE 'fake%') $$;
CREATE OPERATOR public.= (LEFTARG = text, RIGHTARG = int4, FUNCTION = text_eq_int);

CREATE FUNCTION int_eq3(int4, int4) RETURNS boolean LANGUAGE sql
  AS $$ SELECT $1 = $2 $$;
CREATE OPERATOR public.=== (LEFTARG = int4, RIGHTARG = int4, FUNCTION = int_eq3);

CREATE SCHEMA archive;
CREATE TABLE archive.employees (id int PRIMARY KEY, name text NOT NULL DEFAULT '');
INSERT INTO archive.employees VALUES (1, 'Archived Example');

-- The app user the suite connects as: DML on its schemas, use of the sequence, and CREATE on public the way an app that runs its own migrations has it, so a refused SELECT INTO proves the read-only transaction rather than a missing privilege. Not a superuser.
CREATE ROLE vaultapp LOGIN PASSWORD 'vaultapp-pass';
GRANT CONNECT ON DATABASE vaultapp TO vaultapp;
GRANT USAGE, CREATE ON SCHEMA public TO vaultapp;
GRANT USAGE ON SCHEMA archive TO vaultapp;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO vaultapp;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA archive TO vaultapp;
GRANT USAGE, SELECT, UPDATE ON SEQUENCE invoice_seq TO vaultapp;

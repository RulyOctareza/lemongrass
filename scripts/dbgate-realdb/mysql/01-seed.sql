-- SPDX-License-Identifier: AGPL-3.0-only
-- Copyright (C) 2026 Faizal Alviansyah

-- Fake fixtures for internal/dbgate/tricks_realdb_test.go, run as root by the image on a fresh volume, inside vaultapp.
-- A channel may grant employees and salaries; nothing ever grants secrets. Every name here is used by tricks_test.go.

CREATE TABLE employees (id INT PRIMARY KEY, name VARCHAR(100) NOT NULL DEFAULT '');
CREATE TABLE salaries (employee_id INT PRIMARY KEY, amount INT NOT NULL);
CREATE TABLE secrets (id INT PRIMARY KEY, secret VARCHAR(100) NOT NULL);
-- MySQL 8 has no sequences, so this counter stands in for one.
CREATE TABLE invoice_counter (id INT PRIMARY KEY, next_value BIGINT NOT NULL);

INSERT INTO employees VALUES (1, 'Ada Example'), (2, 'Bo Sample'), (3, 'Cy Placeholder');
INSERT INTO salaries VALUES (1, 5000), (2, 6000), (3, 7000);
INSERT INTO secrets VALUES (1, 'fake-secret-alpha'), (2, 'fake-secret-beta');
INSERT INTO invoice_counter VALUES (1, 100);

CREATE VIEW secret_view AS SELECT id, secret FROM secrets;

CREATE FUNCTION read_secrets() RETURNS TEXT READS SQL DATA
  RETURN (SELECT GROUP_CONCAT(secret ORDER BY id) FROM secrets);

-- Stored functions sharing a native name: sum is reached by `sum (x)` or a qualified call, format_pico_time on any server that lacks the native one.
CREATE FUNCTION `sum`(x BIGINT) RETURNS TEXT READS SQL DATA
  RETURN (SELECT GROUP_CONCAT(secret ORDER BY id) FROM secrets);
CREATE FUNCTION `format_pico_time`(x BIGINT) RETURNS TEXT READS SQL DATA
  RETURN (SELECT GROUP_CONCAT(secret ORDER BY id) FROM secrets);

DELIMITER //
CREATE FUNCTION bump_salaries() RETURNS INT MODIFIES SQL DATA
BEGIN
  UPDATE salaries SET amount = amount + 1;
  RETURN 1;
END//
CREATE FUNCTION next_invoice() RETURNS BIGINT MODIFIES SQL DATA
BEGIN
  UPDATE invoice_counter SET next_value = next_value + 1 WHERE id = 1;
  RETURN (SELECT next_value FROM invoice_counter WHERE id = 1);
END//
CREATE PROCEDURE bump_proc()
BEGIN
  UPDATE salaries SET amount = amount + 1;
END//
DELIMITER ;

CREATE DATABASE vault_archive;
CREATE TABLE vault_archive.employees (id INT PRIMARY KEY, name VARCHAR(100) NOT NULL DEFAULT '');
INSERT INTO vault_archive.employees VALUES (1, 'Archived Example');

-- The app user the suite connects as: DML and EXECUTE on its own databases, CREATE and DROP on vaultapp the way an app that runs its own migrations has them (service_mysql_test.go builds its own tables), plus what the performance views need (PROCESS and reads of performance_schema and sys). No FILE, no SUPER.
CREATE USER 'vaultapp'@'%' IDENTIFIED BY 'vaultapp-pass';
GRANT SELECT, INSERT, UPDATE, DELETE, EXECUTE, CREATE, DROP ON vaultapp.* TO 'vaultapp'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON vault_archive.* TO 'vaultapp'@'%';
GRANT SELECT ON performance_schema.* TO 'vaultapp'@'%';
GRANT SELECT, EXECUTE ON sys.* TO 'vaultapp'@'%';
GRANT PROCESS ON *.* TO 'vaultapp'@'%';

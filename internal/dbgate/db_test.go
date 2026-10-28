// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"testing"
	"time"
)

func TestEngineOf(t *testing.T) {
	cases := []struct {
		conn string
		want Engine
	}{
		{"mysql://user:pass@host:3306/db", EngineMySQL},
		{"mariadb://user:pass@host:3306/db", EngineMySQL},
		{"postgres://user:pass@host:5432/db", EnginePostgres},
		{"postgresql://user:pass@host:5432/db", EnginePostgres},
	}
	for _, c := range cases {
		got, err := engineOf(c.conn)
		if err != nil {
			t.Errorf("engineOf(%q): %v", c.conn, err)
			continue
		}
		if got != c.want {
			t.Errorf("engineOf(%q) = %q, want %q", c.conn, got, c.want)
		}
	}
}

func TestEngineOfRejectsUnsupportedOrMalformed(t *testing.T) {
	for _, conn := range []string{"sqlite:///db.sqlite", "not-a-connection-string", ""} {
		if _, err := engineOf(conn); err == nil {
			t.Errorf("engineOf(%q): expected an error, got nil", conn)
		}
	}
}

func TestMysqlDSN(t *testing.T) {
	got, err := mysqlDSN("mysql://root:secret@127.0.0.1:3306/appdb")
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	want := "root:secret@tcp(127.0.0.1:3306)/appdb"
	if got != want {
		t.Errorf("mysqlDSN = %q, want %q", got, want)
	}
}

func TestMysqlDSNNoCredentials(t *testing.T) {
	got, err := mysqlDSN("mysql://127.0.0.1:3306/appdb")
	if err != nil {
		t.Fatalf("mysqlDSN: %v", err)
	}
	want := "tcp(127.0.0.1:3306)/appdb"
	if got != want {
		t.Errorf("mysqlDSN = %q, want %q", got, want)
	}
}

func TestMysqlDSNRejectsNoHost(t *testing.T) {
	if _, err := mysqlDSN("mysql:///appdb"); err == nil {
		t.Error("mysqlDSN with no host: expected an error, got nil")
	}
}

func TestNormalizeValue(t *testing.T) {
	when := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{"nil", nil, nil},
		{"bool", true, true},
		{"int64", int64(42), int64(42)},
		{"float64", float64(3.5), float64(3.5)},
		{"string", "alice", "alice"},
		{"time", when, "2026-09-11T12:00:00Z"},
		{"bytes", []byte("blob"), "blob"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeValue(c.in); got != c.want {
				t.Errorf("normalizeValue(%#v) = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

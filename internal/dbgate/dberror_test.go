// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDescribeDBErrorScrubsAccountsAndAddresses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want []string
		gone []string
	}{
		{
			"mysql table denied",
			&mysql.MySQLError{Number: 1142, Message: "SELECT command denied to user 'app_user'@'203.0.113.7' for table 'events'"},
			[]string{"1142", "SELECT command denied to the database user for table 'events'"},
			[]string{"app_user", "203.0.113.7"},
		},
		{
			"mysql access denied",
			&mysql.MySQLError{Number: 1045, Message: "Access denied for user 'app_user'@'host.example.com' (using password: YES)"},
			[]string{"1045", "Access denied for the database user (using password: YES)"},
			[]string{"app_user", "host.example.com"},
		},
		{
			"mysql host not allowed",
			&mysql.MySQLError{Number: 1130, Message: "Host '203.0.113.7' is not allowed to connect to this MySQL server"},
			[]string{"1130", "is not allowed to connect"},
			[]string{"203.0.113.7"},
		},
		{
			"mysql definer",
			&mysql.MySQLError{Number: 1449, Message: "The user specified as a definer ('root'@'%') does not exist"},
			[]string{"1449", "does not exist"},
			[]string{"root"},
		},
		{
			"mysql keeps table and column detail",
			&mysql.MySQLError{Number: 1146, Message: "Table 'appdb.orders' doesn't exist"},
			[]string{"1146", "Table 'appdb.orders' doesn't exist"},
			nil,
		},
		{
			"postgres role",
			fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "28P01", Message: `password authentication failed for user "app_user"`}),
			[]string{"28P01", "password authentication failed for the database user"},
			[]string{"app_user"},
		},
		{
			"postgres permission",
			&pgconn.PgError{Code: "42501", Message: "permission denied for view pg_stat_statements"},
			[]string{"42501", "permission denied for view pg_stat_statements"},
			nil,
		},
		{
			"unreachable server",
			&net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 3306}, Err: errors.New("connect: connection refused")},
			[]string{"could not be reached"},
			[]string{"203.0.113.7", "3306"},
		},
		{"deadline", fmt.Errorf("x: %w", context.DeadlineExceeded), []string{"did not answer in time"}, nil},
		{"invalid connection", mysql.ErrInvalidConn, []string{"could not be reached"}, nil},
		{"unknown", errors.New("driver blew up talking to 203.0.113.7 as app_user"), []string{"could not be completed"}, []string{"203.0.113.7", "app_user"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := describeDBError(c.err)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("describeDBError = %q, want it to contain %q", got, w)
				}
			}
			for _, g := range c.gone {
				if strings.Contains(got, g) {
					t.Errorf("describeDBError = %q, must not contain %q", got, g)
				}
			}
		})
	}
}

func TestServiceQueryErrorDoesNotEchoTheConnectionTarget(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	_, err = svc.Query(c.ID, "SELECT id FROM employees")
	wantsExecution(t, err)
	for _, leaked := range []string{"127.0.0.1", "root", "secret"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("error %q must not contain %q", err, leaked)
		}
	}
}

func TestConnectionStringErrorsDoNotEchoTheString(t *testing.T) {
	if _, err := engineOf("root:hunter2@host/db"); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("engineOf error = %v, want a failure without the string", err)
	}
	if _, err := mysqlDSN("mysql://root:hunter2%zz@host/db"); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("mysqlDSN parse error = %v, want a failure without the string", err)
	}
	if _, err := mysqlDSN("mysql:///db"); err == nil || strings.Contains(err.Error(), "db") && strings.Contains(err.Error(), "mysql:///") {
		t.Errorf("mysqlDSN no-host error = %v, want a failure without the string", err)
	}
}

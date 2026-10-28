// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	mysqlUserRegex   = regexp.MustCompile(`(?i)\buser '[^']*'@'[^']*'`)
	pairRegex        = regexp.MustCompile(`'[^']*'@'[^']*'`)
	hostRegex        = regexp.MustCompile(`(?i)\bhost '[^']*'`)
	postgresRoleText = regexp.MustCompile(`(?i)\b(?:user|role) "[^"]*"`)
	ipv4Regex        = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
)

// describeDBError turns a driver error into text safe to show a model: server errors keep their code and message with account names and addresses removed, and anything else collapses to a generic sentence.
func describeDBError(err error) string {
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		return fmt.Sprintf("database error %d: %s", myErr.Number, scrubDBMessage(myErr.Message))
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Sprintf("database error %s: %s", pgErr.Code, scrubDBMessage(pgErr.Message))
	}
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "the database did not answer in time"
	case errors.As(err, &netErr), errors.Is(err, mysql.ErrInvalidConn):
		return "the database server could not be reached"
	default:
		return "the query could not be completed"
	}
}

func scrubDBMessage(msg string) string {
	msg = mysqlUserRegex.ReplaceAllString(msg, "the database user")
	msg = postgresRoleText.ReplaceAllString(msg, "the database user")
	msg = pairRegex.ReplaceAllString(msg, "'***'@'***'")
	msg = hostRegex.ReplaceAllString(msg, "host '***'")
	return ipv4Regex.ReplaceAllString(msg, "***")
}

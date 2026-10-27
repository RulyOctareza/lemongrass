// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"errors"
	"strings"

	"github.com/xwb1989/sqlparser"
)

var (
	errMySQLDashComment  = errors.New(`dbgate: "--" starts a comment only when a space follows it, add the space or remove the dashes`)
	errMySQLSlashComment = errors.New(`dbgate: "//" is not a comment in MySQL`)
	errMySQLHint         = errors.New("dbgate: optimizer hint comments (/*+ ... */) are not allowed")
	errMySQLMariaDBCode  = errors.New("dbgate: MariaDB executable comments (/*M! ... */) are not allowed")
	errMySQLOpaque       = errors.New("dbgate: this SHOW, DESCRIBE or EXPLAIN form may not hold a subquery, a function call or a WHERE clause")
	errMySQLReadLocks    = errors.New("dbgate: a read may not lock rows with FOR UPDATE or LOCK IN SHARE MODE")
)

// checkMySQLComments refuses comments the parser drops but MySQL reads as code. The parser ends a "--" comment at the line end whatever follows the dashes, while MySQL needs whitespace after them, so `SELECT 1 --1, (SELECT ...)` hides a subquery from the parser. It also treats "//" as a comment, which MySQL doesn't, and drops /*+ */ hints, which MySQL acts on, and /*M! */, which MariaDB runs as code. "#", "--" followed by whitespace and /*!NNNNN */ agree: the parser reads /*! */ content as code whatever its version, so it only ever sees more than the server runs.
func checkMySQLComments(sqlText string) error {
	tkn := sqlparser.NewStringTokenizer(sqlText)
	for {
		typ, val := tkn.Scan()
		switch typ {
		case 0, sqlparser.LEX_ERROR:
			return nil
		case sqlparser.COMMENT:
			c := string(val)
			switch {
			case strings.HasPrefix(c, "--"):
				if len(c) == 2 || !strings.ContainsRune(" \t\r\n", rune(c[2])) {
					return errMySQLDashComment
				}
			case strings.HasPrefix(c, "//"):
				return errMySQLSlashComment
			case strings.HasPrefix(c, "/*+"):
				return errMySQLHint
			case strings.HasPrefix(c, "/*M!"):
				return errMySQLMariaDBCode
			}
		}
	}
}

// checkMySQLOpaque refuses a statement the parser cut short with force_eof, most SHOW forms, DESCRIBE and an EXPLAIN whose query it can't read, when the unparsed text could hold an expression: a SELECT, a WHERE, or a "(" not closed at once. MySQL evaluates that text, but no table or function check ever saw it. An empty "()" stays allowed for SHOW GRANTS FOR CURRENT_USER().
func checkMySQLOpaque(sqlText string) error {
	tkn := sqlparser.NewStringTokenizer(sqlText)
	prevOpen := false
	for {
		typ, _ := tkn.Scan()
		if typ == sqlparser.COMMENT {
			continue
		}
		if prevOpen && typ != ')' {
			return errMySQLOpaque
		}
		prevOpen = false
		switch typ {
		case 0:
			return nil
		case sqlparser.LEX_ERROR, sqlparser.SELECT, sqlparser.WHERE:
			return errMySQLOpaque
		case '(':
			prevOpen = true
		}
	}
}

// checkMySQLReadLocks refuses a read that takes row locks, matching Postgres, where the read-only transaction refuses them too.
func checkMySQLReadLocks(stmt sqlparser.SQLNode) error {
	return mysqlWalk(func(n sqlparser.SQLNode) (bool, error) {
		switch s := n.(type) {
		case *sqlparser.Select:
			if s.Lock != "" {
				return false, errMySQLReadLocks
			}
		case *sqlparser.Union:
			if s.Lock != "" {
				return false, errMySQLReadLocks
			}
		}
		return true, nil
	}, stmt)
}

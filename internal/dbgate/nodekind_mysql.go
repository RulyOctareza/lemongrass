// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/xwb1989/sqlparser"
)

// mysqlSyntaxNodes are the parse-tree node kinds that do nothing on their own, so the walk only goes into their children. Any kind not listed here or handled in checkMySQLNode refuses the statement, so a node a newer parser adds fails closed instead of passing unchecked.
var mysqlSyntaxNodes = mysqlNodeKinds(
	(*sqlparser.Select)(nil), (*sqlparser.Union)(nil), (*sqlparser.ParenSelect)(nil),
	sqlparser.SelectExprs(nil), (*sqlparser.AliasedExpr)(nil), (*sqlparser.StarExpr)(nil),
	sqlparser.TableExprs(nil), (*sqlparser.AliasedTableExpr)(nil), (*sqlparser.ParenTableExpr)(nil),
	(*sqlparser.JoinTableExpr)(nil), sqlparser.JoinCondition{}, sqlparser.TableName{},
	sqlparser.TableNames(nil), sqlparser.TableIdent{}, sqlparser.ColIdent{}, (*sqlparser.IndexHints)(nil),
	sqlparser.Partitions(nil), (*sqlparser.Where)(nil), sqlparser.GroupBy(nil), sqlparser.OrderBy(nil),
	(*sqlparser.Order)(nil), (*sqlparser.Limit)(nil), sqlparser.Comments(nil), sqlparser.Exprs(nil),
	(*sqlparser.AndExpr)(nil), (*sqlparser.OrExpr)(nil), (*sqlparser.NotExpr)(nil), (*sqlparser.ParenExpr)(nil),
	(*sqlparser.ComparisonExpr)(nil), (*sqlparser.RangeCond)(nil), (*sqlparser.IsExpr)(nil),
	(*sqlparser.ExistsExpr)(nil), (*sqlparser.Subquery)(nil), (*sqlparser.BinaryExpr)(nil),
	(*sqlparser.UnaryExpr)(nil), (*sqlparser.IntervalExpr)(nil), (*sqlparser.CollateExpr)(nil),
	(*sqlparser.CaseExpr)(nil), (*sqlparser.When)(nil), (*sqlparser.ColName)(nil), sqlparser.ValTuple(nil),
	(*sqlparser.NullVal)(nil), sqlparser.BoolVal(false), (*sqlparser.ConvertExpr)(nil),
	(*sqlparser.ConvertType)(nil), (*sqlparser.ConvertUsingExpr)(nil), (*sqlparser.SubstrExpr)(nil),
	(*sqlparser.MatchExpr)(nil), (*sqlparser.GroupConcatExpr)(nil), (*sqlparser.ValuesFuncExpr)(nil),
	(*sqlparser.Default)(nil), (*sqlparser.Insert)(nil), sqlparser.Values(nil), sqlparser.Columns(nil),
	sqlparser.OnDup(nil), (*sqlparser.Update)(nil), sqlparser.UpdateExprs(nil), (*sqlparser.UpdateExpr)(nil),
	(*sqlparser.Delete)(nil),
)

func mysqlNodeKinds(samples ...sqlparser.SQLNode) map[reflect.Type]bool {
	kinds := make(map[reflect.Type]bool, len(samples))
	for _, s := range samples {
		kinds[reflect.TypeOf(s)] = true
	}
	return kinds
}

// checkMySQLNode refuses a node kind that is neither plain syntax nor one the caller checks itself. A SQLVal bind variable (? or :name) is refused here: it is a Vitess placeholder that MySQL never fills from a single statement's text.
func checkMySQLNode(n sqlparser.SQLNode) error {
	switch v := n.(type) {
	case *sqlparser.FuncExpr:
		return nil
	case *sqlparser.SQLVal:
		if v.Type == sqlparser.ValArg {
			return fmt.Errorf("dbgate: bind variable %s is not supported", v.Val)
		}
		return nil
	}
	if mysqlSyntaxNodes[reflect.TypeOf(n)] {
		return nil
	}
	name := strings.TrimPrefix(strings.TrimPrefix(reflect.TypeOf(n).String(), "*"), "sqlparser.")
	return fmt.Errorf("dbgate: %s is not supported", name)
}

// mysqlWalk is sqlparser.Walk plus the children the parser's own walk skips: a UNION's ORDER BY and LIMIT, which can hold subqueries and function calls. Every MySQL check walks through it, so none of them misses those.
func mysqlWalk(visit sqlparser.Visit, nodes ...sqlparser.SQLNode) error {
	return sqlparser.Walk(func(n sqlparser.SQLNode) (bool, error) {
		kontinue, err := visit(n)
		if err != nil || !kontinue {
			return kontinue, err
		}
		if u, ok := n.(*sqlparser.Union); ok && u != nil {
			if err := mysqlWalk(visit, u.OrderBy, u.Limit); err != nil {
				return false, err
			}
		}
		return true, nil
	}, nodes...)
}

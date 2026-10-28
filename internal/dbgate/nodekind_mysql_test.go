// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xwb1989/sqlparser"
)

func TestMySQLNodeKindsRefuseUnknown(t *testing.T) {
	for _, n := range []sqlparser.SQLNode{
		sqlparser.Nextval{Expr: sqlparser.NewIntVal([]byte("1"))},
		sqlparser.ListArg("::list"),
		&sqlparser.Set{},
		sqlparser.SetExprs(nil),
		&sqlparser.DDL{Action: "create"},
		&sqlparser.Use{},
		&sqlparser.OtherAdmin{},
		&sqlparser.Show{Type: "tables"},
		&sqlparser.Stream{},
		sqlparser.NewValArg([]byte(":v1")),
	} {
		if err := checkMySQLNode(n); err == nil {
			t.Errorf("checkMySQLNode(%T) = nil, want a refusal", n)
		}
	}
	for _, sql := range []string{"SELECT ? FROM employees", "SELECT id FROM employees WHERE id = :id"} {
		_, err := ClassifyQuery(EngineMySQL, sql)
		if err == nil || !strings.Contains(err.Error(), "bind variable") {
			t.Errorf("ClassifyQuery(%q) = %v, want the bind variable refusal", sql, err)
		}
	}
}

func TestMySQLNodeKindsAcceptOrdinaryQueries(t *testing.T) {
	for _, sql := range []string{
		"SELECT DISTINCT e.id, e.name AS n, 1, 2.5, 'x', NULL, TRUE, -e.id, e.id + 1, e.id BETWEEN 1 AND 2, e.name IS NOT NULL, (e.id, e.id) = (1, 1) FROM employees AS e USE INDEX (PRIMARY) WHERE e.id IN (1, 2) AND NOT (e.name LIKE 'a%' OR e.name COLLATE utf8mb4_bin = 'b') GROUP BY e.id, e.name HAVING COUNT(*) > 0 ORDER BY e.id DESC LIMIT 10 OFFSET 5",
		"SELECT * FROM employees e LEFT JOIN (SELECT id FROM employees) d ON d.id = e.id JOIN employees f USING (id) WHERE EXISTS (SELECT 1 FROM employees) UNION ALL (SELECT * FROM employees)",
		"SELECT CASE e.id WHEN 1 THEN 'a' ELSE 'b' END, CAST(e.id AS CHAR(10)), CONVERT(e.name USING utf8mb4), SUBSTRING(e.name, 1, 2), GROUP_CONCAT(e.name), DATE_ADD(e.hired, INTERVAL 1 DAY) FROM employees e WHERE MATCH(e.name) AGAINST ('x')",
		"INSERT INTO employees (id, name) VALUES (1, 'a'), (2, DEFAULT) ON DUPLICATE KEY UPDATE name = VALUES(name)",
		"INSERT INTO employees (id) SELECT id FROM employees WHERE id = 1",
		"UPDATE employees SET name = 'x', id = id + 1 WHERE id = 1 ORDER BY id LIMIT 1",
		"DELETE FROM employees WHERE id = 1 ORDER BY id LIMIT 1",
		"DELETE e FROM employees e JOIN employees f ON e.id = f.id WHERE f.id = 1",
	} {
		if _, err := ClassifyQuery(EngineMySQL, sql); err != nil {
			t.Errorf("ClassifyQuery(%q): %v", sql, err)
		}
	}
}

// TestMySQLSyntaxNodesWalkTheirChildren fills every node-typed field of each accepted kind with a fresh marker node and checks mysqlWalk reaches it. An accepted kind whose walk skips a child would leave that child unchecked, as Show's empty walk and Union's skipped ORDER BY and LIMIT once did.
func TestMySQLSyntaxNodesWalkTheirChildren(t *testing.T) {
	for typ := range mysqlSyntaxNodes {
		node := reflect.New(typ).Elem()
		var markers []reflect.Value
		fillMarkers(node, &markers, 2)
		if len(markers) == 0 {
			continue
		}
		visited := make(map[uintptr]bool)
		root, ok := addressable(node).Interface().(sqlparser.SQLNode)
		if !ok {
			t.Errorf("%s is not a SQLNode", typ)
			continue
		}
		mysqlWalk(func(n sqlparser.SQLNode) (bool, error) {
			if v := reflect.ValueOf(n); v.Kind() == reflect.Ptr && !v.IsNil() {
				visited[v.Pointer()] = true
			}
			return true, nil
		}, root)
		for _, m := range markers {
			if !visited[m.Pointer()] {
				t.Errorf("Walk of %s never visits its %s child", typ, m.Type())
			}
		}
	}
}

// addressable returns v itself for a pointer, which reflect.New(typ).Elem() already holds, and v for a value type.
func addressable(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Ptr && v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return v
}

var (
	sqlNodeType = reflect.TypeOf((*sqlparser.SQLNode)(nil)).Elem()
	// markerFactories build a fresh pointer node per call, so each marker has its own identity.
	markerFactories = []func() sqlparser.SQLNode{
		func() sqlparser.SQLNode { return &sqlparser.ColName{} },
		func() sqlparser.SQLNode { return &sqlparser.Select{} },
		func() sqlparser.SQLNode { return &sqlparser.AliasedExpr{} },
		func() sqlparser.SQLNode { return &sqlparser.AliasedTableExpr{} },
		func() sqlparser.SQLNode { return &sqlparser.Subquery{} },
	}
)

// sqlValType is skipped as a field type: only ConvertType's Length and Scale hold one directly, and the grammar fills them with integer literals alone.
var sqlValType = reflect.TypeOf((*sqlparser.SQLVal)(nil))

// marker returns a fresh pointer node assignable to t, or an invalid Value when none fits. An empty interface, such as ColName's Metadata, is planner data rather than a child.
func marker(t reflect.Type) reflect.Value {
	if t == sqlValType {
		return reflect.Value{}
	}
	if t.Kind() == reflect.Ptr && t.Implements(sqlNodeType) {
		return reflect.New(t.Elem())
	}
	if t.Kind() == reflect.Interface && t.NumMethod() > 0 {
		for _, f := range markerFactories {
			if v := reflect.ValueOf(f()); v.Type().AssignableTo(t) {
				return v
			}
		}
	}
	return reflect.Value{}
}

// fillMarkers sets every node-typed field reachable from v, through value structs up to depth levels, to a marker, and records the markers.
func fillMarkers(v reflect.Value, markers *[]reflect.Value, depth int) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			setMarker(f, markers, depth)
		}
	case reflect.Slice:
		setMarker(v, markers, depth)
	}
}

func setMarker(f reflect.Value, markers *[]reflect.Value, depth int) {
	switch f.Kind() {
	case reflect.Ptr, reflect.Interface:
		if m := marker(f.Type()); m.IsValid() {
			f.Set(m)
			*markers = append(*markers, m)
		}
	case reflect.Slice:
		if m := marker(f.Type().Elem()); m.IsValid() {
			s := reflect.MakeSlice(f.Type(), 1, 1)
			s.Index(0).Set(m)
			f.Set(s)
			*markers = append(*markers, m)
		}
	case reflect.Struct:
		if depth > 0 {
			fillMarkers(f, markers, depth-1)
		}
	}
}

func TestMySQLUnionOrderByAndLimitAreChecked(t *testing.T) {
	stmt := classifyFor(t, EngineMySQL, "SELECT id FROM employees UNION SELECT id FROM employees ORDER BY (SELECT secret FROM salaries LIMIT 1)")
	if !reflect.DeepEqual(stmt.Tables, []string{"employees", "salaries"}) {
		t.Errorf("Tables = %v, want the ORDER BY subquery's salaries too", stmt.Tables)
	}
	if _, err := ClassifyQuery(EngineMySQL, "SELECT id FROM employees UNION SELECT id FROM employees ORDER BY UPPER(`sleep`(1))"); err == nil {
		t.Error("a call in a UNION's ORDER BY passed unchecked")
	}
}

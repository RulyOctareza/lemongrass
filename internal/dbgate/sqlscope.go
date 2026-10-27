// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	// pgquery parses via WASM (no cgo) into pganalyze's own parse-tree types.
	pganalyze "github.com/pganalyze/pg_query_go/v6"
	pgquery "github.com/wasilibs/go-pgquery"
	"github.com/xwb1989/sqlparser"
)

// Kind classifies what a statement does, independent of engine.
type Kind string

const (
	KindSelect Kind = "select"
	// KindExplain is scope-checked against the tables of the query it wraps, not granted separately.
	KindExplain Kind = "explain"
	// KindIntrospect is SHOW/DESCRIBE-shaped schema or server metadata, table-scope checked only when a specific table is recoverable.
	KindIntrospect Kind = "introspect"
	KindInsert     Kind = "insert"
	KindUpdate     Kind = "update"
	KindDelete     Kind = "delete"
)

// Statement holds what a piece of SQL actually does and which real tables it references, the ground truth scope is checked against.
type Statement struct {
	Kind   Kind
	Engine Engine
	Tables []string
	// WriteTables is the subset of Tables this statement actually writes to, empty for every read kind.
	WriteTables []string
	// ListsTables marks a SHOW TABLES-shaped statement, whose result rows name every table in the database rather than one this Statement itself references.
	ListsTables bool
}

// ClassifyQuery accepts exactly one statement of a supported read-only kind, rejecting stacked statements outright rather than truncating to the first.
func ClassifyQuery(engine Engine, sqlText string) (Statement, error) {
	var stmt Statement
	var err error
	switch engine {
	case EngineMySQL:
		stmt, err = classifyMySQL(sqlText)
	case EnginePostgres:
		stmt, err = classifyPostgres(sqlText)
	default:
		return Statement{}, fmt.Errorf("dbgate: unsupported engine %q", engine)
	}
	stmt.Engine = engine
	return stmt, err
}

func classifyMySQL(sqlText string) (Statement, error) {
	pieces, err := sqlparser.SplitStatementToPieces(sqlText)
	if err != nil {
		return Statement{}, fmt.Errorf("dbgate: splitting SQL: %w", err)
	}
	var stmts []string
	for _, p := range pieces {
		if strings.TrimSpace(p) != "" {
			stmts = append(stmts, p)
		}
	}
	if len(stmts) != 1 {
		return Statement{}, fmt.Errorf("dbgate: exactly one SQL statement is allowed, got %d", len(stmts))
	}

	if stmt, ok, err := classifyMySQLExplain(stmts[0]); ok || err != nil {
		return stmt, err
	}

	stmt, err := sqlparser.Parse(stmts[0])
	if err != nil {
		return Statement{}, fmt.Errorf("dbgate: parsing SQL: %w", err)
	}

	switch st := stmt.(type) {
	case *sqlparser.Select, *sqlparser.Union, *sqlparser.ParenSelect:
		tables := mysqlTables(stmt)
		if err := checkRestrictedColumns(EngineMySQL, tables, mysqlColumnRefs(stmt)); err != nil {
			return Statement{}, err
		}
		return Statement{Kind: KindSelect, Tables: tables}, nil
	case *sqlparser.Show:
		if st.HasOnTable() {
			return Statement{Kind: KindIntrospect, Tables: []string{st.OnTable.Name.String()}}, nil
		}
		if st.Type == "tables" {
			return Statement{Kind: KindIntrospect, ListsTables: true}, nil
		}
		return Statement{Kind: KindIntrospect, Tables: introspectTableFromText(stmts[0])}, nil
	case *sqlparser.OtherRead:
		return Statement{Kind: KindIntrospect, Tables: introspectTableFromText(stmts[0])}, nil
	case *sqlparser.Insert:
		return classifyMySQLInsert(st)
	case *sqlparser.Update:
		return classifyMySQLUpdate(st)
	case *sqlparser.Delete:
		return classifyMySQLDelete(st)
	default:
		return Statement{}, fmt.Errorf("dbgate: %T is not a supported statement", stmt)
	}
}

// classifyMySQLInsert always writes exactly one table, the parser's own dedicated Table field rather than an AliasedTableExpr, so it isn't picked up by mysqlTables and is added to Tables separately.
func classifyMySQLInsert(stmt *sqlparser.Insert) (Statement, error) {
	target := qualifiedTable(EngineMySQL, stmt.Table.Qualifier.String(), stmt.Table.Name.String())
	tables := toSet(mysqlTables(stmt))
	tables[target] = true
	return Statement{Kind: KindInsert, Tables: sortedKeys(tables), WriteTables: []string{target}}, nil
}

func classifyMySQLUpdate(stmt *sqlparser.Update) (Statement, error) {
	if stmt.Where == nil {
		return Statement{}, fmt.Errorf("dbgate: UPDATE without a WHERE clause is not allowed")
	}
	tables := mysqlTables(stmt)
	return Statement{Kind: KindUpdate, Tables: tables, WriteTables: mysqlUpdateWriteTables(stmt, tables)}, nil
}

// mysqlUpdateWriteTables resolves which of the update's tables are actually written to. A single-table update is unambiguous; a multi-table one (UPDATE a, b SET a.x = b.x ...) is resolved by tracing each SET column's table qualifier, alias or real name, back to a real table via mysqlTableAliases.
func mysqlUpdateWriteTables(stmt *sqlparser.Update, tables []string) []string {
	if len(tables) == 1 {
		return tables
	}
	aliases := mysqlTableAliases(stmt.TableExprs)
	seen := make(map[string]bool)
	for _, e := range stmt.Exprs {
		q := e.Name.Qualifier
		if q.IsEmpty() {
			continue
		}
		if schema := q.Qualifier.String(); schema != "" {
			seen[qualifiedTable(EngineMySQL, schema, q.Name.String())] = true
			continue
		}
		if real, ok := aliases[strings.ToLower(q.Name.String())]; ok {
			seen[real] = true
		}
	}
	return sortedKeys(seen)
}

func classifyMySQLDelete(stmt *sqlparser.Delete) (Statement, error) {
	if stmt.Where == nil {
		return Statement{}, fmt.Errorf("dbgate: DELETE without a WHERE clause is not allowed")
	}
	tables := mysqlTables(stmt)
	if len(stmt.Targets) == 0 {
		return Statement{Kind: KindDelete, Tables: tables, WriteTables: tables}, nil
	}
	seen := make(map[string]bool)
	for _, tn := range stmt.Targets {
		seen[qualifiedTable(EngineMySQL, tn.Qualifier.String(), tn.Name.String())] = true
	}
	return Statement{Kind: KindDelete, Tables: tables, WriteTables: sortedKeys(seen)}, nil
}

// mysqlTableAliases maps each table's alias, and its real name, to its qualified table name, for resolving a SET column's qualifier in a multi-table UPDATE back to a real table.
func mysqlTableAliases(node sqlparser.SQLNode) map[string]string {
	aliases := make(map[string]string)
	sqlparser.Walk(func(n sqlparser.SQLNode) (bool, error) {
		if ate, ok := n.(*sqlparser.AliasedTableExpr); ok {
			if tn, ok := ate.Expr.(sqlparser.TableName); ok && !tn.IsEmpty() {
				qualified := qualifiedTable(EngineMySQL, tn.Qualifier.String(), tn.Name.String())
				aliases[strings.ToLower(tn.Name.String())] = qualified
				if !ate.As.IsEmpty() {
					aliases[strings.ToLower(ate.As.String())] = qualified
				}
			}
		}
		return true, nil
	}, node)
	return aliases
}

var errExplainNotReadOnly = errors.New("dbgate: EXPLAIN is only allowed on a SELECT that writes nothing")

// mysqlExplainRegex splits EXPLAIN/DESCRIBE/DESC and its known modifiers from the statement it wraps, which this parser's grammar force_eofs instead of parsing.
var mysqlExplainRegex = regexp.MustCompile(`(?is)^\s*(?:EXPLAIN|DESCRIBE|DESC)((?:\s+(?:ANALYZE|EXTENDED|PARTITIONS|FORMAT\s*=\s*\w+))*)\s+(.*)$`)

var (
	mysqlWriteLeadRegex  = regexp.MustCompile(`(?i)^(?:INSERT|REPLACE|UPDATE|DELETE)\b`)
	mysqlSelectLeadRegex = regexp.MustCompile(`(?i)^(?:SELECT\b|\()`)
)

// classifyMySQLExplain parses the SELECT an EXPLAIN wraps so it is checked as explain on that SELECT's own tables. ok is false for any form it doesn't recognize, or a wrapped query this parser can't read, leaving it to the table-less introspect path.
func classifyMySQLExplain(sqlText string) (Statement, bool, error) {
	m := mysqlExplainRegex.FindStringSubmatch(sqlText)
	if m == nil {
		return Statement{}, false, nil
	}
	inner := m[2]
	if mysqlWriteLeadRegex.MatchString(inner) {
		return Statement{}, false, errExplainNotReadOnly
	}
	if !mysqlSelectLeadRegex.MatchString(inner) {
		return Statement{}, false, nil
	}
	stmt, err := sqlparser.Parse(inner)
	if err != nil {
		return Statement{}, false, nil
	}
	switch stmt.(type) {
	case *sqlparser.Select, *sqlparser.Union, *sqlparser.ParenSelect:
	default:
		return Statement{}, false, nil
	}
	tables := mysqlTables(stmt)
	if err := checkRestrictedColumns(EngineMySQL, tables, mysqlColumnRefs(stmt)); err != nil {
		return Statement{}, false, err
	}
	return Statement{Kind: KindExplain, Tables: tables}, true, nil
}

// introspectTableRegex recovers the table this parser force_eofs out of DESCRIBE/DESC and SHOW CREATE TABLE/COLUMNS/FIELDS/INDEX/INDEXES/KEYS.
var introspectTableRegex = regexp.MustCompile(`(?is)^\s*(?:DESCRIBE|DESC|SHOW\s+CREATE\s+TABLE|SHOW\s+(?:COLUMNS|FIELDS|INDEX|INDEXES|KEYS)\s+(?:FROM|IN))\s+` + "`?([a-zA-Z0-9_$]+(?:\\.[a-zA-Z0-9_$]+)?)`?")

func introspectTableFromText(sqlText string) []string {
	m := introspectTableRegex.FindStringSubmatch(sqlText)
	if m == nil {
		return nil
	}
	return []string{m[1]}
}

// mysqlTables collects TableName nodes only via AliasedTableExpr, since TableName is reused for column qualifiers and walking it directly would count aliases as tables.
func mysqlTables(stmt sqlparser.SQLNode) []string {
	seen := make(map[string]bool)
	sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		if ate, ok := node.(*sqlparser.AliasedTableExpr); ok {
			if tn, ok := ate.Expr.(sqlparser.TableName); ok && !tn.IsEmpty() {
				seen[qualifiedTable(EngineMySQL, tn.Qualifier.String(), tn.Name.String())] = true
			}
		}
		return true, nil
	}, stmt)
	return sortedKeys(seen)
}

// mysqlColumnRefs collects every column name in stmt and whether a select list uses a star, leaving out the star inside an aggregate such as COUNT(*).
func mysqlColumnRefs(stmt sqlparser.SQLNode) columnRefs {
	refs := columnRefs{names: make(map[string]bool)}
	aggregateStars := make(map[*sqlparser.StarExpr]bool)
	sqlparser.Walk(func(node sqlparser.SQLNode) (bool, error) {
		switch n := node.(type) {
		case *sqlparser.FuncExpr:
			for _, e := range n.Exprs {
				if star, ok := e.(*sqlparser.StarExpr); ok {
					aggregateStars[star] = true
				}
			}
		case *sqlparser.ColName:
			refs.names[n.Name.Lowered()] = true
		case *sqlparser.StarExpr:
			if !aggregateStars[n] {
				refs.star = true
			}
		}
		return true, nil
	}, stmt)
	return refs
}

func classifyPostgres(sqlText string) (Statement, error) {
	tree, err := pgquery.Parse(sqlText)
	if err != nil {
		return Statement{}, fmt.Errorf("dbgate: parsing SQL: %w", err)
	}
	if len(tree.Stmts) != 1 {
		return Statement{}, fmt.Errorf("dbgate: exactly one SQL statement is allowed, got %d", len(tree.Stmts))
	}

	var kind Kind
	// writeRelation is the statement's single write target, set only for insert/update/delete, always alone since Postgres names exactly one relation per write statement even when FROM/USING references others.
	var writeRelation *pganalyze.RangeVar
	switch st := tree.Stmts[0].Stmt.Node.(type) {
	case *pganalyze.Node_SelectStmt:
		kind = KindSelect
	case *pganalyze.Node_ExplainStmt:
		// EXPLAIN ANALYZE executes the statement it wraps, so anything but a SELECT would let an explain grant write.
		if _, ok := st.ExplainStmt.GetQuery().GetNode().(*pganalyze.Node_SelectStmt); !ok {
			return Statement{}, errExplainNotReadOnly
		}
		kind = KindExplain
	case *pganalyze.Node_VariableShowStmt:
		return Statement{Kind: KindIntrospect}, nil
	case *pganalyze.Node_InsertStmt:
		kind = KindInsert
		writeRelation = st.InsertStmt.Relation
	case *pganalyze.Node_UpdateStmt:
		if st.UpdateStmt.WhereClause == nil {
			return Statement{}, fmt.Errorf("dbgate: UPDATE without a WHERE clause is not allowed")
		}
		kind = KindUpdate
		writeRelation = st.UpdateStmt.Relation
	case *pganalyze.Node_DeleteStmt:
		if st.DeleteStmt.WhereClause == nil {
			return Statement{}, fmt.Errorf("dbgate: DELETE without a WHERE clause is not allowed")
		}
		kind = KindDelete
		writeRelation = st.DeleteStmt.Relation
	default:
		return Statement{}, fmt.Errorf("dbgate: %T is not a supported statement", tree.Stmts[0].Stmt.Node)
	}

	parsed, err := postgresJSON(sqlText)
	if err != nil {
		return Statement{}, err
	}
	if kind == KindExplain && postgresWrites(parsed) {
		return Statement{}, errExplainNotReadOnly
	}
	tables := postgresTables(parsed)

	if writeRelation != nil {
		// Relation is a direct *RangeVar field, not a Node-oneof wrapped in a "RangeVar" JSON key like FromClause/UsingClause entries are, so postgresTables never sees it and it's folded in here instead.
		write := qualifiedTable(EnginePostgres, writeRelation.Schemaname, writeRelation.Relname)
		all := toSet(tables)
		all[write] = true
		return Statement{Kind: kind, Tables: sortedKeys(all), WriteTables: []string{write}}, nil
	}

	if err := checkRestrictedColumns(EnginePostgres, tables, postgresColumnRefs(parsed)); err != nil {
		return Statement{}, err
	}
	return Statement{Kind: kind, Tables: tables}, nil
}

func postgresJSON(sqlText string) (any, error) {
	js, err := pgquery.ParseToJSON(sqlText)
	if err != nil {
		return nil, fmt.Errorf("dbgate: parsing SQL: %w", err)
	}
	var tree any
	if err := json.Unmarshal([]byte(js), &tree); err != nil {
		return nil, fmt.Errorf("dbgate: decoding parsed SQL: %w", err)
	}
	return tree, nil
}

// postgresWrites reports whether a SELECT's tree writes anywhere, through a data-modifying CTE or SELECT INTO, neither of which postgresTables sees as a table.
func postgresWrites(tree any) bool {
	found := false
	walkJSON(tree, func(key string, _ any) {
		switch key {
		case "InsertStmt", "UpdateStmt", "DeleteStmt", "MergeStmt", "intoClause":
			found = true
		}
	})
	return found
}

// postgresTables collects every RangeVar's relname except the statement's own CTE names, since Postgres uses RangeVar only for real relations, unlike MySQL's reused TableName.
func postgresTables(tree any) []string {
	ctes := make(map[string]bool)
	walkJSON(tree, func(key string, val any) {
		if key != "CommonTableExpr" {
			return
		}
		if m, ok := val.(map[string]any); ok {
			if name, ok := m["ctename"].(string); ok {
				ctes[name] = true
			}
		}
	})

	tables := make(map[string]bool)
	walkJSON(tree, func(key string, val any) {
		if key != "RangeVar" {
			return
		}
		if m, ok := val.(map[string]any); ok {
			name, _ := m["relname"].(string)
			schema, _ := m["schemaname"].(string)
			if name != "" && !ctes[name] {
				tables[qualifiedTable(EnginePostgres, schema, name)] = true
			}
		}
	})
	return sortedKeys(tables)
}

// postgresColumnRefs collects the last name of every ColumnRef and whether any of them is a star.
func postgresColumnRefs(tree any) columnRefs {
	refs := columnRefs{names: make(map[string]bool)}
	walkJSON(tree, func(key string, val any) {
		if key != "ColumnRef" {
			return
		}
		m, ok := val.(map[string]any)
		if !ok {
			return
		}
		fields, _ := m["fields"].([]any)
		for _, f := range fields {
			node, ok := f.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := node["A_Star"]; ok {
				refs.star = true
			}
			if str, ok := node["String"].(map[string]any); ok {
				if name, ok := str["sval"].(string); ok {
					refs.names[strings.ToLower(name)] = true
				}
			}
		}
	})
	return refs
}

// walkJSON depth-first visits every key/value pair in a tree decoded from JSON via
// json.Unmarshal into `any`.
func walkJSON(node any, visit func(key string, val any)) {
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			visit(k, val)
			walkJSON(val, visit)
		}
	case []any:
		for _, item := range v {
			walkJSON(item, visit)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toSet(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, v := range list {
		m[v] = true
	}
	return m
}

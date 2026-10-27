// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

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
	// Catalog holds the names whose resolution depends on objects in the database, looked up in the same transaction before the statement runs.
	Catalog CatalogNames
}

// CatalogNames lists what a statement could resolve to a user object the parser cannot see: called functions on both engines, plus attribute-notation names and operator symbols on Postgres.
type CatalogNames struct {
	Functions  []string
	Attributes []string
	Operators  []string
}

// ClassifyQuery accepts exactly one statement of a supported read-only kind, rejecting stacked statements outright rather than truncating to the first.
func ClassifyQuery(engine Engine, sqlText string) (Statement, error) {
	// A parser that reads a C string, like libpg_query, stops at the first NUL, so anything after it would reach the server unread.
	if strings.ContainsRune(sqlText, 0) {
		return Statement{}, errors.New("dbgate: SQL may not contain a NUL byte")
	}
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
	if err := checkMySQLComments(sqlText); err != nil {
		return Statement{}, err
	}
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
		return classifyMySQLRead(KindSelect, stmts[0], stmt)
	case *sqlparser.Show:
		if filter := mysqlShowTablesFilter(st); filter != nil {
			return classifyMySQLShowTablesFilter(stmts[0], filter)
		}
		// Show.walkSubtree visits nothing, and most SHOW forms keep their tail only as text.
		if err := checkMySQLOpaque(stmts[0]); err != nil {
			return Statement{}, err
		}
		if st.HasOnTable() {
			return Statement{Kind: KindIntrospect, Tables: []string{st.OnTable.Name.String()}}, nil
		}
		if st.Type == "tables" {
			return Statement{Kind: KindIntrospect, ListsTables: true}, nil
		}
		return Statement{Kind: KindIntrospect, Tables: introspectTableFromText(stmts[0])}, nil
	case *sqlparser.OtherRead:
		if err := checkMySQLOpaque(stmts[0]); err != nil {
			return Statement{}, err
		}
		return Statement{Kind: KindIntrospect, Tables: introspectTableFromText(stmts[0])}, nil
	case *sqlparser.Insert:
		return classifyMySQLInsert(stmts[0], st)
	case *sqlparser.Update:
		return classifyMySQLUpdate(stmts[0], st)
	case *sqlparser.Delete:
		return classifyMySQLDelete(stmts[0], st)
	default:
		return Statement{}, fmt.Errorf("dbgate: %T is not a supported statement", stmt)
	}
}

// classifyMySQLRead checks a SELECT-shaped read, top-level or wrapped by EXPLAIN, and returns it as kind.
func classifyMySQLRead(kind Kind, sqlText string, stmt sqlparser.SQLNode) (Statement, error) {
	if err := checkMySQLReadLocks(stmt); err != nil {
		return Statement{}, err
	}
	calls, err := mysqlCalls(sqlText, stmt)
	if err != nil {
		return Statement{}, err
	}
	tables := mysqlTables(sqlText, stmt)
	if err := checkRestrictedColumns(EngineMySQL, tables, mysqlColumnRefs(stmt)); err != nil {
		return Statement{}, err
	}
	return Statement{Kind: kind, Tables: tables, Catalog: CatalogNames{Functions: calls}}, nil
}

// mysqlShowTablesFilter returns the parsed WHERE expression of SHOW TABLES, the one SHOW form whose filter the parser keeps as a tree.
func mysqlShowTablesFilter(st *sqlparser.Show) sqlparser.Expr {
	if st.ShowTablesOpt == nil || st.ShowTablesOpt.Filter == nil {
		return nil
	}
	return st.ShowTablesOpt.Filter.Filter
}

// classifyMySQLShowTablesFilter checks a SHOW TABLES WHERE filter like a read, so a table its subquery names needs a grant and its calls the allowlist.
func classifyMySQLShowTablesFilter(sqlText string, filter sqlparser.Expr) (Statement, error) {
	if err := checkMySQLReadLocks(filter); err != nil {
		return Statement{}, err
	}
	calls, err := mysqlCalls(sqlText, filter)
	if err != nil {
		return Statement{}, err
	}
	tables := mysqlTables(sqlText, filter)
	if err := checkRestrictedColumns(EngineMySQL, tables, mysqlColumnRefs(filter)); err != nil {
		return Statement{}, err
	}
	return Statement{Kind: KindIntrospect, Tables: tables, ListsTables: true, Catalog: CatalogNames{Functions: calls}}, nil
}

// classifyMySQLInsert always writes exactly one table, the parser's own dedicated Table field rather than an AliasedTableExpr, so it isn't picked up by mysqlTables and is added to Tables separately.
func classifyMySQLInsert(sqlText string, stmt *sqlparser.Insert) (Statement, error) {
	calls, err := mysqlCalls(sqlText, stmt)
	if err != nil {
		return Statement{}, err
	}
	target := qualifiedTable(EngineMySQL, stmt.Table.Qualifier.String(), stmt.Table.Name.String())
	tables := toSet(mysqlTables(sqlText, stmt))
	tables[target] = true
	return Statement{Kind: KindInsert, Tables: sortedKeys(tables), WriteTables: []string{target}, Catalog: CatalogNames{Functions: calls}}, nil
}

func classifyMySQLUpdate(sqlText string, stmt *sqlparser.Update) (Statement, error) {
	if stmt.Where == nil {
		return Statement{}, fmt.Errorf("dbgate: UPDATE without a WHERE clause is not allowed")
	}
	calls, err := mysqlCalls(sqlText, stmt)
	if err != nil {
		return Statement{}, err
	}
	tables := mysqlTables(sqlText, stmt)
	writes, err := mysqlUpdateWriteTables(stmt, tables)
	if err != nil {
		return Statement{}, err
	}
	return Statement{Kind: KindUpdate, Tables: tables, WriteTables: writes, Catalog: CatalogNames{Functions: calls}}, nil
}

// mysqlUpdateWriteTables resolves which of the update's tables are actually written to. A single-table update is unambiguous; a multi-table one (UPDATE a, b SET a.x = b.x ...) is resolved by tracing each SET column's table qualifier, alias or real name, back to a real table via mysqlTableRefs.
func mysqlUpdateWriteTables(stmt *sqlparser.Update, tables []string) ([]string, error) {
	if len(tables) == 1 {
		return tables, nil
	}
	aliases, err := mysqlTableRefs(stmt.TableExprs)
	if err != nil {
		return nil, err
	}
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
	return sortedKeys(seen), nil
}

func classifyMySQLDelete(sqlText string, stmt *sqlparser.Delete) (Statement, error) {
	if stmt.Where == nil {
		return Statement{}, fmt.Errorf("dbgate: DELETE without a WHERE clause is not allowed")
	}
	calls, err := mysqlCalls(sqlText, stmt)
	if err != nil {
		return Statement{}, err
	}
	catalog := CatalogNames{Functions: calls}
	tables := mysqlTables(sqlText, stmt)
	if len(stmt.Targets) == 0 {
		return Statement{Kind: KindDelete, Tables: tables, WriteTables: tables, Catalog: catalog}, nil
	}
	refs, err := mysqlTableRefs(stmt.TableExprs)
	if err != nil {
		return Statement{}, err
	}
	all := toSet(tables)
	seen := make(map[string]bool)
	for _, tn := range stmt.Targets {
		var real string
		if tn.Qualifier.IsEmpty() {
			real = refs[strings.ToLower(tn.Name.String())]
		} else if q := qualifiedTable(EngineMySQL, tn.Qualifier.String(), tn.Name.String()); all[q] {
			real = q
		}
		// MySQL itself rejects a target that names no table in the FROM list, so refusing one loses nothing.
		if real == "" {
			return Statement{}, fmt.Errorf("dbgate: DELETE target %s does not name a table in the statement", sqlparser.String(tn))
		}
		seen[real] = true
	}
	return Statement{Kind: KindDelete, Tables: tables, WriteTables: sortedKeys(seen), Catalog: catalog}, nil
}

// mysqlTableRefs maps the name each table in a multi-table UPDATE or DELETE's own table list answers to, its alias when it has one and its real name otherwise, to its qualified table name, which is how MySQL resolves a DELETE target or a SET column qualifier. Subqueries are skipped, since their tables are out of reach of both. A name two different tables answer to, compared case-insensitively since alias case sensitivity depends on the server, is refused as ambiguous.
func mysqlTableRefs(exprs sqlparser.TableExprs) (map[string]string, error) {
	refs := make(map[string]string)
	err := mysqlWalk(func(n sqlparser.SQLNode) (bool, error) {
		switch node := n.(type) {
		case *sqlparser.Subquery:
			return false, nil
		case *sqlparser.AliasedTableExpr:
			tn, ok := node.Expr.(sqlparser.TableName)
			if !ok || tn.IsEmpty() {
				return true, nil
			}
			qualified := qualifiedTable(EngineMySQL, tn.Qualifier.String(), tn.Name.String())
			name := tn.Name.String()
			if !node.As.IsEmpty() {
				name = node.As.String()
			}
			key := strings.ToLower(name)
			if prev, ok := refs[key]; ok && prev != qualified {
				return false, fmt.Errorf("dbgate: table name %s is ambiguous in this statement", name)
			}
			refs[key] = qualified
		}
		return true, nil
	}, exprs)
	return refs, err
}

var errExplainNotReadOnly = errors.New("dbgate: EXPLAIN is only allowed on a SELECT that writes nothing")

// mysqlExplainRegex splits EXPLAIN/DESCRIBE/DESC and its known modifiers from the statement it wraps, which this parser's grammar force_eofs instead of parsing.
var mysqlExplainRegex = regexp.MustCompile(`(?is)^\s*(?:EXPLAIN|DESCRIBE|DESC)((?:\s+(?:ANALYZE|EXTENDED|PARTITIONS|FORMAT\s*=\s*\w+))*)\s+(.*)$`)

var (
	mysqlWriteLeadRegex  = regexp.MustCompile(`(?i)^(?:INSERT|REPLACE|UPDATE|DELETE)\b`)
	mysqlSelectLeadRegex = regexp.MustCompile(`(?i)^(?:SELECT\b|\()`)
)

// classifyMySQLExplain parses the SELECT an EXPLAIN wraps so it is checked as explain on that SELECT's own tables. ok is false for any form it doesn't recognize, or a wrapped query this parser can't read, leaving it to the table-less introspect path, where checkMySQLOpaque refuses any such query that holds an expression.
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
	read, err := classifyMySQLRead(KindExplain, inner, stmt)
	if err != nil {
		return Statement{}, false, err
	}
	return read, true, nil
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

// mysqlTables collects TableName nodes only via AliasedTableExpr, since TableName is reused for column qualifiers and walking it directly would count aliases as tables. An unqualified dual is MySQL's dummy table, which the parser also adds to a FROM-less SELECT, and is left out unless sqlText backticks it, the only way to name a real table called dual.
func mysqlTables(sqlText string, stmt sqlparser.SQLNode) []string {
	keepDual := mysqlBacktickedDual(sqlText)
	seen := make(map[string]bool)
	mysqlWalk(func(node sqlparser.SQLNode) (bool, error) {
		if ate, ok := node.(*sqlparser.AliasedTableExpr); ok {
			if tn, ok := ate.Expr.(sqlparser.TableName); ok && !tn.IsEmpty() {
				if !keepDual && tn.Qualifier.IsEmpty() && strings.EqualFold(tn.Name.String(), "dual") {
					return true, nil
				}
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
	mysqlWalk(func(node sqlparser.SQLNode) (bool, error) {
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
	writes := postgresWriteCount(parsed)
	if writeRelation == nil {
		if writes > 0 && kind == KindExplain {
			return Statement{}, errExplainNotReadOnly
		}
		if writes > 0 {
			return Statement{}, errPostgresReadWrites
		}
		if postgresLocks(parsed) {
			return Statement{}, errPostgresReadLocks
		}
	} else if writes != 1 {
		// Only the top-level statement's own relation reaches WriteTables, so a nested write would change a table no scope check sees.
		return Statement{}, errPostgresNestedWrite
	}
	catalog, err := checkPostgresCalls(parsed)
	if err != nil {
		return Statement{}, err
	}
	tables := postgresTables(parsed)

	if writeRelation != nil {
		// Relation is a direct *RangeVar field, not a Node-oneof wrapped in a "RangeVar" JSON key like FromClause/UsingClause entries are, so postgresTables never sees it and it's folded in here instead.
		write := qualifiedTable(EnginePostgres, writeRelation.Schemaname, writeRelation.Relname)
		all := toSet(tables)
		all[write] = true
		return Statement{Kind: kind, Tables: sortedKeys(all), WriteTables: []string{write}, Catalog: catalog}, nil
	}

	if err := checkRestrictedColumns(EnginePostgres, tables, postgresColumnRefs(parsed)); err != nil {
		return Statement{}, err
	}
	return Statement{Kind: kind, Tables: tables, Catalog: catalog}, nil
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

var (
	errPostgresReadWrites  = errors.New("dbgate: a read may not write, through a data-modifying WITH or SELECT INTO")
	errPostgresReadLocks   = errors.New("dbgate: a read may not lock rows with FOR UPDATE or FOR SHARE")
	errPostgresNestedWrite = errors.New("dbgate: a write statement may not hold another write, such as a data-modifying WITH")
)

// postgresWriteCount counts every write in the tree, the top-level statement included, since a data-modifying CTE or SELECT INTO writes through a relation postgresTables never sees.
func postgresWriteCount(tree any) int {
	n := 0
	walkJSON(tree, func(key string, _ any) {
		switch key {
		case "InsertStmt", "UpdateStmt", "DeleteStmt", "MergeStmt", "intoClause":
			n++
		}
	})
	return n
}

// postgresLocks reports whether any SELECT in the tree takes row locks with FOR UPDATE/SHARE.
func postgresLocks(tree any) bool {
	found := false
	walkJSON(tree, func(key string, _ any) {
		if key == "lockingClause" || key == "LockingClause" {
			found = true
		}
	})
	return found
}

// postgresTables collects every RangeVar's relation except a bare name that resolves to a CTE at that point in the tree, since Postgres uses RangeVar only for real relations, unlike MySQL's reused TableName.
func postgresTables(tree any) []string {
	tables := make(map[string]bool)
	collectPostgresTables(tree, nil, tables)
	return sortedKeys(tables)
}

// collectPostgresTables walks node with the CTE names visible there, following Postgres's scoping: a WITH's names cover its own statement and every subquery under it, and inside the CTE queries themselves only earlier siblings, or all of them under RECURSIVE. A name it cannot prove is a CTE counts as a table, so a mistake can only refuse.
func collectPostgresTables(node any, visible map[string]bool, tables map[string]bool) {
	switch v := node.(type) {
	case []any:
		for _, item := range v {
			collectPostgresTables(item, visible, tables)
		}
	case map[string]any:
		if with, ok := v["withClause"].(map[string]any); ok {
			visible = collectPostgresWith(with, visible, tables)
		}
		for key, val := range v {
			switch key {
			case "withClause":
			case "RangeVar":
				m, _ := val.(map[string]any)
				name, _ := m["relname"].(string)
				schema, _ := m["schemaname"].(string)
				// A schema-qualified name never refers to a CTE.
				if name != "" && (schema != "" || !visible[name]) {
					tables[qualifiedTable(EnginePostgres, schema, name)] = true
				}
			default:
				collectPostgresTables(val, visible, tables)
			}
		}
	}
}

// collectPostgresWith walks a WITH clause's CTE queries and returns the names visible in the statement body it belongs to.
func collectPostgresWith(with map[string]any, outer map[string]bool, tables map[string]bool) map[string]bool {
	recursive, _ := with["recursive"].(bool)
	ctes, _ := with["ctes"].([]any)
	var names []string
	var queries []any
	for _, item := range ctes {
		node, _ := item.(map[string]any)
		cte, _ := node["CommonTableExpr"].(map[string]any)
		name, _ := cte["ctename"].(string)
		names = append(names, name)
		queries = append(queries, cte)
	}
	withNames := func(n int) map[string]bool {
		out := make(map[string]bool, len(outer)+n)
		for k := range outer {
			out[k] = true
		}
		for _, name := range names[:n] {
			out[name] = true
		}
		return out
	}
	for i, cte := range queries {
		seen := i
		if recursive {
			seen = len(names)
		}
		collectPostgresTables(cte, withNames(seen), tables)
	}
	return withNames(len(names))
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

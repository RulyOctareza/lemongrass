// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"fmt"
	"strings"
)

// postgresAllowedFunctions is every function a Postgres statement may call: built-ins with no side effects that read no table, matched by bare name once the schema is checked to be absent or pg_catalog. Anything missing, user functions included, is refused, since only an allowlist can be complete.
var postgresAllowedFunctions = toSet([]string{
	// aggregates and window functions
	"count", "sum", "avg", "min", "max", "bool_and", "bool_or", "every", "bit_and", "bit_or", "bit_xor",
	"string_agg", "array_agg", "json_agg", "jsonb_agg", "json_object_agg", "jsonb_object_agg", "xmlagg",
	"stddev", "stddev_pop", "stddev_samp", "variance", "var_pop", "var_samp",
	"corr", "covar_pop", "covar_samp", "regr_avgx", "regr_avgy", "regr_count", "regr_intercept", "regr_r2", "regr_slope", "regr_sxx", "regr_sxy", "regr_syy",
	"percentile_cont", "percentile_disc", "mode", "grouping",
	"row_number", "rank", "dense_rank", "percent_rank", "cume_dist", "ntile", "lag", "lead", "first_value", "last_value", "nth_value",
	// string
	"lower", "upper", "initcap", "length", "char_length", "character_length", "octet_length", "bit_length",
	"substr", "substring", "btrim", "ltrim", "rtrim", "trim", "position", "strpos", "replace", "translate", "overlay",
	"concat", "concat_ws", "format", "left", "right", "lpad", "rpad", "repeat", "reverse", "split_part", "starts_with",
	"regexp_match", "regexp_matches", "regexp_replace", "regexp_split_to_array", "regexp_split_to_table", "regexp_like", "regexp_count", "regexp_instr", "regexp_substr",
	"similar_to_escape", "normalize", "is_normalized", "string_to_array", "string_to_table", "array_to_string",
	"ascii", "chr", "to_hex", "encode", "decode", "convert_from", "convert_to", "quote_ident", "quote_literal", "quote_nullable",
	"md5", "sha224", "sha256", "sha384", "sha512",
	// math
	"abs", "ceil", "ceiling", "floor", "round", "trunc", "mod", "power", "pow", "sqrt", "cbrt", "exp", "ln", "log", "log10", "sign", "div",
	"pi", "degrees", "radians", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "cot", "sinh", "cosh", "tanh",
	"width_bucket", "gcd", "lcm", "scale", "min_scale", "trim_scale", "factorial",
	// date and time
	"now", "clock_timestamp", "statement_timestamp", "transaction_timestamp", "timeofday",
	"date_trunc", "date_part", "date_bin", "extract", "age", "timezone", "isfinite",
	"make_date", "make_time", "make_timestamp", "make_timestamptz", "make_interval", "justify_days", "justify_hours", "justify_interval",
	"to_char", "to_date", "to_timestamp", "to_number",
	// JSON
	"to_json", "to_jsonb", "row_to_json", "array_to_json", "json_build_array", "jsonb_build_array", "json_build_object", "jsonb_build_object",
	"json_object", "jsonb_object", "json_array_length", "jsonb_array_length", "json_each", "jsonb_each", "json_each_text", "jsonb_each_text",
	"json_extract_path", "jsonb_extract_path", "json_extract_path_text", "jsonb_extract_path_text", "json_object_keys", "jsonb_object_keys",
	"json_array_elements", "jsonb_array_elements", "json_array_elements_text", "jsonb_array_elements_text", "json_typeof", "jsonb_typeof",
	"json_strip_nulls", "jsonb_strip_nulls", "jsonb_pretty", "jsonb_set", "jsonb_set_lax", "jsonb_insert",
	"jsonb_path_query", "jsonb_path_query_array", "jsonb_path_query_first", "jsonb_path_exists", "jsonb_path_match",
	"json_populate_record", "jsonb_populate_record", "json_to_record", "jsonb_to_record", "json_to_recordset", "jsonb_to_recordset",
	// arrays and set-returning
	"array_length", "array_lower", "array_upper", "array_ndims", "array_dims", "cardinality", "array_position", "array_positions",
	"array_remove", "array_replace", "array_append", "array_prepend", "array_cat", "array_fill", "trim_array",
	"unnest", "generate_series", "generate_subscripts",
	// XML built-ins that read no table, unlike query_to_xml and table_to_xml
	"xmlcomment", "xpath", "xpath_exists", "xml_is_well_formed", "xml_is_well_formed_document", "xml_is_well_formed_content",
	// text search
	"to_tsvector", "to_tsquery", "plainto_tsquery", "phraseto_tsquery", "websearch_to_tsquery", "ts_rank", "ts_rank_cd", "ts_headline",
	// conditionals, casting and misc
	"num_nulls", "num_nonnulls", "pg_typeof", "pg_collation_for", "gen_random_uuid", "pg_size_pretty", "pg_size_bytes",
})

// postgresOperators is every operator symbol a statement may use, plus the keyword names the parser gives BETWEEN forms. A user operator can only shadow these through search_path, which the vault pins with pg_catalog first.
var postgresOperators = toSet([]string{
	"=", "<>", "!=", "<", ">", "<=", ">=", "+", "-", "*", "/", "%", "^", "|/", "||/", "@", "&", "|", "#", "~", "<<", ">>",
	"||", "~~", "!~~", "~~*", "!~~*", "!~", "~*", "!~*",
	"->", "->>", "#>", "#>>", "@>", "<@", "?", "?|", "?&", "#-", "@?", "@@", "&&",
	"BETWEEN", "NOT BETWEEN", "BETWEEN SYMMETRIC", "NOT BETWEEN SYMMETRIC",
})

// postgresTypes is every type a cast or column definition may name. A user type's input function and a user cast are user code, so they are refused like a user function.
var postgresTypes = toSet([]string{
	"bool", "boolean", "int2", "int4", "int8", "smallint", "integer", "int", "bigint", "numeric", "decimal", "float4", "float8", "real",
	"text", "varchar", "bpchar", "char", "name", "bytea", "uuid", "json", "jsonb", "jsonpath", "xml",
	"date", "time", "timetz", "timestamp", "timestamptz", "interval",
	"inet", "cidr", "macaddr", "macaddr8", "bit", "varbit", "tsvector", "tsquery", "oid", "money",
})

// postgresTableSamples is every TABLESAMPLE method a statement may use, since a method is a handler function that could be user code.
var postgresTableSamples = toSet([]string{"system", "bernoulli"})

// checkPostgresCalls refuses a statement whose tree calls a function, operator, type or tablesample method outside its allowlist, wherever it sits, the FROM clause and ROWS FROM included. It returns the names the catalog check must still look up: a pg_catalog-qualified name is left out, since it resolves only inside pg_catalog.
func checkPostgresCalls(tree any) (CatalogNames, error) {
	functions := make(map[string]bool)
	attributes := make(map[string]bool)
	operators := make(map[string]bool)
	var err error
	walkJSON(tree, func(key string, val any) {
		if err != nil {
			return
		}
		m, _ := val.(map[string]any)
		switch key {
		case "FuncCall":
			err = checkPostgresName("function", m["funcname"], postgresAllowedFunctions, functions)
		case "A_Expr":
			err = checkPostgresName("operator", m["name"], postgresOperators, operators)
		case "operName", "useOp":
			// SubLink's `x op ANY (subquery)` and SortBy's `ORDER BY x USING op` name an operator outside A_Expr.
			err = checkPostgresName("operator", val, postgresOperators, operators)
		case "typeName", "TypeName":
			err = checkPostgresName("type", m["names"], postgresTypes, nil)
		case "RangeTableSample":
			err = checkPostgresName("tablesample method", m["method"], postgresTableSamples, nil)
		case "ColumnRef":
			// Attribute notation: `e.leak` calls leak(e) when e has no column leak, and with a composite column any later field can be such a call too.
			fields := postgresNameParts(m["fields"])
			for i := 1; i < len(fields); i++ {
				if fields[i] != "" {
					attributes[fields[i]] = true
				}
			}
		case "A_Indirection":
			for _, name := range postgresNameParts(m["indirection"]) {
				if name != "" {
					attributes[name] = true
				}
			}
		}
	})
	if err != nil {
		return CatalogNames{}, err
	}
	// BETWEEN forms call >= and <= under the hood, so those are the symbols a user operator could shadow.
	for _, kw := range []string{"BETWEEN", "NOT BETWEEN", "BETWEEN SYMMETRIC", "NOT BETWEEN SYMMETRIC"} {
		if operators[kw] {
			delete(operators, kw)
			operators[">="] = true
			operators["<="] = true
		}
	}
	return CatalogNames{Functions: nonEmptyKeys(functions), Attributes: nonEmptyKeys(attributes), Operators: nonEmptyKeys(operators)}, nil
}

// nonEmptyKeys is sortedKeys that leaves a Statement with nothing to look up equal to its zero value.
func nonEmptyKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	return sortedKeys(m)
}

// checkPostgresName accepts a parsed name list that is a bare allowlisted name or that name qualified by pg_catalog, so a same-named object in another schema never passes. A bare name is added to lookup, when given, since search_path resolution can still reach a user object for it.
func checkPostgresName(what string, list any, allowed map[string]bool, lookup map[string]bool) error {
	parts := postgresNameParts(list)
	switch {
	case len(parts) == 1 && allowed[parts[0]]:
		if lookup != nil {
			lookup[parts[0]] = true
		}
		return nil
	case len(parts) == 2 && parts[0] == "pg_catalog" && allowed[parts[1]]:
		return nil
	}
	full := strings.Join(parts, ".")
	if what == "function" {
		return &ErrFunctionNotAllowed{Name: full}
	}
	return fmt.Errorf("dbgate: %s %s is not allowed", what, full)
}

// postgresNameParts reads a parsed name list, leaving "" for an entry that is not a String node, such as A_Star or A_Indices.
func postgresNameParts(list any) []string {
	items, _ := list.([]any)
	parts := make([]string, 0, len(items))
	for _, item := range items {
		node, _ := item.(map[string]any)
		str, _ := node["String"].(map[string]any)
		name, _ := str["sval"].(string)
		parts = append(parts, name)
	}
	return parts
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package dbgate

import (
	"fmt"
	"regexp"

	"github.com/xwb1989/sqlparser"
)

// mysqlAllowedFunctions is every function a MySQL statement may call by name: built-ins with no side effects that read no table, lowercase since MySQL function names are case-insensitive. CAST, CONVERT, CASE, GROUP_CONCAT, SUBSTRING, MATCH ... AGAINST and VALUES() are parsed into their own node types and allowed as forms, not listed here. SLEEP, BENCHMARK, the GET_LOCK family, LOAD_FILE, LAST_INSERT_ID (which sets the session value when given an argument) and UUID_SHORT (which bumps a server counter) stay out.
var mysqlAllowedFunctions = toSet([]string{
	// aggregates
	"avg", "bit_and", "bit_or", "bit_xor", "count", "max", "min", "std", "stddev", "stddev_pop",
	"stddev_samp", "sum", "var_pop", "var_samp", "variance", "json_arrayagg", "json_objectagg",
	// string
	"ascii", "bin", "bit_length", "char", "char_length", "character_length", "concat", "concat_ws",
	"elt", "export_set", "field", "find_in_set", "format", "from_base64", "hex", "instr", "lcase",
	"left", "length", "locate", "lower", "lpad", "ltrim", "make_set", "mid", "oct", "octet_length",
	"ord", "quote", "regexp_instr", "regexp_like", "regexp_replace", "regexp_substr", "repeat",
	"replace", "reverse", "right", "rpad", "rtrim", "soundex", "space", "strcmp", "substring_index",
	"to_base64", "ucase", "unhex", "upper",
	// math
	"abs", "acos", "asin", "atan", "atan2", "ceil", "ceiling", "conv", "cos", "cot", "crc32",
	"degrees", "exp", "floor", "ln", "log", "log10", "log2", "mod", "pi", "pow", "power", "radians",
	"rand", "round", "sign", "sin", "sqrt", "tan", "truncate",
	// date and time
	"adddate", "addtime", "convert_tz", "curdate", "current_date", "current_time",
	"current_timestamp", "curtime", "date", "date_add", "date_format", "date_sub", "datediff", "day",
	"dayname", "dayofmonth", "dayofweek", "dayofyear", "from_days", "from_unixtime", "get_format",
	"hour", "last_day", "localtime", "localtimestamp", "makedate", "maketime", "microsecond",
	"minute", "month", "monthname", "now", "period_add", "period_diff", "quarter", "sec_to_time",
	"second", "str_to_date", "subdate", "subtime", "sysdate", "time", "time_format", "time_to_sec",
	"timediff", "timestamp", "timestampadd", "timestampdiff", "to_days", "to_seconds",
	"unix_timestamp", "utc_date", "utc_time", "utc_timestamp", "week", "weekday", "weekofyear",
	"year", "yearweek",
	// JSON, each returning a value rather than writing one
	"json_array", "json_array_append", "json_array_insert", "json_contains", "json_contains_path",
	"json_depth", "json_extract", "json_insert", "json_keys", "json_length", "json_merge_patch",
	"json_merge_preserve", "json_object", "json_overlaps", "json_pretty", "json_quote",
	"json_remove", "json_replace", "json_search", "json_set", "json_type", "json_unquote",
	"json_valid", "json_value",
	// conditionals
	"if", "ifnull", "nullif", "coalesce", "greatest", "least", "isnull",
	// charset and collation
	"charset", "collation", "coercibility",
	// hashing, identifiers and session information
	"md5", "sha", "sha1", "sha2", "uuid", "uuid_to_bin", "bin_to_uuid", "is_uuid", "inet_aton",
	"inet_ntoa", "inet6_aton", "inet6_ntoa", "is_ipv4", "is_ipv6", "database", "schema", "version",
	"connection_id", "current_user", "user", "session_user", "system_user", "found_rows",
	// performance-view formatting, built in since MySQL 8.0.16 in place of the sys schema's stored functions
	"format_pico_time", "format_bytes", "ps_thread_id", "ps_current_thread_id",
})

// mysqlCalls refuses any function call in node that isn't an unqualified allowlisted built-in, checks sqlText spells every call the way MySQL resolves to the built-in, and returns the allowlisted names called, for the catalog check.
func mysqlCalls(sqlText string, node sqlparser.SQLNode) ([]string, error) {
	called := make(map[string]bool)
	// names also holds the node forms, which are grammar keywords and so can't reach a stored function unless misspelled.
	names := make(map[string]bool)
	err := sqlparser.Walk(func(n sqlparser.SQLNode) (bool, error) {
		switch f := n.(type) {
		case *sqlparser.FuncExpr:
			// A MySQL built-in can't be qualified, so a qualified call always names a stored function.
			if !f.Qualifier.IsEmpty() {
				return false, &ErrFunctionNotAllowed{Name: f.Qualifier.String() + "." + f.Name.String()}
			}
			name := f.Name.Lowered()
			if !functionAllowed(EngineMySQL, name) {
				return false, &ErrFunctionNotAllowed{Name: name}
			}
			called[name] = true
			names[name] = true
		case *sqlparser.ConvertExpr:
			names["cast"] = true
		case *sqlparser.SubstrExpr:
			names["substr"] = true
			names["substring"] = true
		case *sqlparser.GroupConcatExpr:
			names["group_concat"] = true
		}
		return true, nil
	}, node)
	if err != nil {
		return nil, err
	}
	if err := checkMySQLCallSpelling(sqlText, sortedKeys(names)); err != nil {
		return nil, err
	}
	return sortedKeys(called), nil
}

// mysqlCallGap matches what may sit between a name and its "(" without the parser noticing: whitespace and comments of every kind.
const mysqlCallGap = `(?:\s|/\*.*?\*/|#[^\n]*|--[^\n]*)`

// checkMySQLCallSpelling refuses a call to one of names written with backticks or with a gap before "(". MySQL parses many built-ins as grammar keywords, and when one is quoted, or spaced while IGNORE_SPACE is off, it becomes a call to a stored function of that name in the default database instead.
func checkMySQLCallSpelling(sqlText string, names []string) error {
	for _, name := range names {
		q := regexp.QuoteMeta(name)
		re := regexp.MustCompile("(?is)(?:`" + q + "`" + mysqlCallGap + `*|(?:^|[^\w$@.` + "`" + `])` + q + mysqlCallGap + `+)\(`)
		if re.MatchString(sqlText) {
			return fmt.Errorf("dbgate: function %s must be written unquoted with \"(\" right after its name", name)
		}
	}
	return nil
}

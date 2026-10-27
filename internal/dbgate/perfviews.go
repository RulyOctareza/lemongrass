// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

import (
	"sort"
	"strings"
)

// OperationPerformance is the Scope.Operations entry that permits select on the engine's performance-view allowlist.
const OperationPerformance = "performance"

// systemSchemas are the schemas whose qualifier is kept on a table name, since stripping it would let a same-named application table stand in for a system one.
var systemSchemas = map[Engine]map[string]bool{
	EngineMySQL:    {"mysql": true, "information_schema": true, "performance_schema": true, "sys": true},
	EnginePostgres: {"pg_catalog": true, "information_schema": true},
}

// perfViews maps a normalized table name to the columns a statement may not reference, per engine.
var perfViews = map[Engine]map[string][]string{
	EngineMySQL:    mysqlPerfViews(),
	EnginePostgres: postgresPerfViews(),
}

func mysqlPerfViews() map[string][]string {
	views := map[string][]string{
		"performance_schema.events_statements_summary_by_digest":              {"query_sample_text"},
		"performance_schema.events_statements_summary_by_program":             nil,
		"performance_schema.events_statements_summary_global_by_event_name":   nil,
		"performance_schema.events_statements_histogram_by_digest":            nil,
		"performance_schema.events_statements_histogram_global":               nil,
		"performance_schema.events_waits_summary_global_by_event_name":        nil,
		"performance_schema.events_waits_summary_by_instance":                 nil,
		"performance_schema.events_stages_summary_global_by_event_name":       nil,
		"performance_schema.events_transactions_summary_global_by_event_name": nil,
		"performance_schema.table_io_waits_summary_by_table":                  nil,
		"performance_schema.table_io_waits_summary_by_index_usage":            nil,
		"performance_schema.table_lock_waits_summary_by_table":                nil,
		"performance_schema.file_summary_by_event_name":                       nil,
		"performance_schema.file_summary_by_instance":                         nil,
		"performance_schema.memory_summary_global_by_event_name":              nil,
		"performance_schema.metadata_locks":                                   nil,
		"performance_schema.data_lock_waits":                                  nil,
		"performance_schema.data_locks":                                       {"lock_data"},
	}
	for _, name := range []string{
		"statement_analysis",
		"statements_with_full_table_scans",
		"statements_with_temp_tables",
		"statements_with_sorting",
		"statements_with_errors_or_warnings",
		"statements_with_runtimes_in_95th_percentile",
		"schema_index_statistics",
		"schema_table_statistics",
		"schema_tables_with_full_table_scans",
		"host_summary",
		"waits_global_by_latency",
		"io_global_by_file_by_bytes",
		"memory_global_total",
	} {
		views["sys."+name] = nil
		views["sys.x$"+name] = nil
	}
	views["sys.schema_unused_indexes"] = nil
	views["sys.schema_redundant_indexes"] = nil
	return views
}

func postgresPerfViews() map[string][]string {
	return map[string][]string{
		"pg_stat_statements":     nil,
		"pg_stat_database":       nil,
		"pg_stat_user_tables":    nil,
		"pg_stat_user_indexes":   nil,
		"pg_statio_user_tables":  nil,
		"pg_statio_user_indexes": nil,
		"pg_locks":               nil,
	}
}

// PerfViewNames lists engine's allowlisted performance views in sorted order.
func PerfViewNames(engine Engine) []string {
	names := make([]string, 0, len(perfViews[engine]))
	for name := range perfViews[engine] {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PerfViewRestrictedColumns lists the columns barred on each engine's allowlisted view, keyed by view name.
func PerfViewRestrictedColumns(engine Engine) map[string][]string {
	out := make(map[string][]string)
	for name, cols := range perfViews[engine] {
		if len(cols) > 0 {
			out[name] = cols
		}
	}
	return out
}

// perfViewKey normalizes a Statement table name to its allowlist key, folding the pg_catalog qualifier away since Postgres resolves it implicitly.
func perfViewKey(engine Engine, table string) string {
	if engine == EnginePostgres {
		return strings.TrimPrefix(table, "pg_catalog.")
	}
	return table
}

// perfViewRestricted returns the columns barred on table and whether table is on engine's allowlist at all.
func perfViewRestricted(engine Engine, table string) ([]string, bool) {
	restricted, ok := perfViews[engine][perfViewKey(engine, table)]
	return restricted, ok
}

// qualifiedTable builds a Statement table name. Any qualifier is kept, since stripping it would let a grant on a bare table reach a same-named table in another schema or database; a system schema is lowercased so it matches the perf-view allowlist, and any other qualifier needs a grant naming it exactly.
func qualifiedTable(engine Engine, qualifier, name string) string {
	q := strings.ToLower(qualifier)
	if q != "" && systemSchemas[engine][q] {
		return q + "." + strings.ToLower(name)
	}
	if qualifier != "" {
		return qualifier + "." + name
	}
	return name
}

// columnRefs holds every column name a statement mentions and whether it selects with a star.
type columnRefs struct {
	names map[string]bool
	star  bool
}

// checkRestrictedColumns rejects a statement that could read a barred column of an allowlisted view, either by naming it or by selecting with a star that would include it.
func checkRestrictedColumns(engine Engine, tables []string, refs columnRefs) error {
	for _, t := range tables {
		restricted, _ := perfViewRestricted(engine, t)
		for _, col := range restricted {
			if refs.star {
				return &ErrRestrictedColumn{Table: t, Column: col, Star: true}
			}
			if refs.names[col] {
				return &ErrRestrictedColumn{Table: t, Column: col}
			}
		}
	}
	return nil
}

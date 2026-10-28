// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package dbgate

// ErrFunctionNotAllowed reports a statement that calls a function missing from its engine's allowlist, a qualified or user-defined one included.
type ErrFunctionNotAllowed struct {
	Name string
}

func (e *ErrFunctionNotAllowed) Error() string {
	return "dbgate: function " + e.Name + " is not allowed"
}

// allowedFunctions holds each engine's allowlist of built-ins that have no side effects and read no table.
var allowedFunctions = map[Engine]map[string]bool{
	EngineMySQL:    mysqlAllowedFunctions,
	EnginePostgres: postgresAllowedFunctions,
}

// functionAllowed is an exact lookup with no case folding: MySQL lowers names itself because its function names are case-insensitive, while a quoted Postgres "LOWER" names a different function from lower and must not match it.
func functionAllowed(engine Engine, name string) bool {
	return allowedFunctions[engine][name]
}

// FunctionNames lists engine's allowlisted functions in sorted order.
func FunctionNames(engine Engine) []string {
	return sortedKeys(allowedFunctions[engine])
}

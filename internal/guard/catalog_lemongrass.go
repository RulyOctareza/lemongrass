// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

var lemongrassRules = []rule{
	{ID: "keyring", Category: catLemongrass, Description: "reading the OS keyring", Mode: ModeDeny, Locked: true, Forms: []form{
		{Tools: []string{"secret-tool", "keyctl"}},
		{Tools: []string{"kwallet-query", "kwalletcli", "kwalletcli_getpin"}},
	}},
	// input-synthesis is locked because a tool that types or clicks could answer the human's own approval modal.
	{ID: "input-synthesis", Category: catLemongrass, Description: "synthesizing keyboard or mouse input", Mode: ModeDeny, Locked: true, Forms: []form{
		{Tools: []string{"xdotool", "ydotool", "ydotoold", "dotool", "dotoolc", "dotoold", "wtype", "xte", "xvkbd", "xmacroplay", "xmacrorec2", "wlrctl", "evemu-event", "evemu-play", "cliclick", "osascript", "autokey-run"}},
	}},
	{ID: "secret-path", Category: catLemongrass, Description: "access to credentials or lemongrass private state", Mode: ModeDeny, Locked: true},
	{ID: "guard-config", Category: catLemongrass, Description: "changing the agent settings, hooks or the lgrass binaries", Mode: ModeDeny, Locked: true},
	{ID: "dynamic-command", Category: catLemongrass, Description: "the command name is computed at runtime, so it cannot be checked", Mode: ModeDeny, Locked: true},
	{ID: "unparseable-command", Category: catLemongrass, Description: "the shell command could not be parsed, so it cannot be checked", Mode: ModeDeny, Locked: true},
	// cmd and PowerShell text is not POSIX shell, so what such a call runs cannot be checked, like a command the parser refuses.
	{ID: "opaque-shell", Category: catLemongrass, Description: "running cmd, PowerShell or a script host, whose commands cannot be checked", Mode: ModeDeny, Locked: true, Forms: []form{
		{Tools: []string{"cmd", "powershell", "pwsh", "powershell_ise", "cscript", "wscript", "mshta", "wsl", "rundll32", "regsvr32", "msiexec", "invoke-expression", "iex", "start-process", "invoke-command"}, Match: opaqueShell},
	}},
}

var databaseRules = []rule{
	{ID: "db-client", Category: catDatabase, Description: "a direct database client, use lgrass db instead", Mode: ModeDeny, Forms: []form{
		{Tools: []string{"mysql", "mariadb", "psql", "mongosh", "mongo", "redis-cli", "sqlcmd", "pg_dump", "pg_dumpall", "pg_restore", "mysqldump", "mysqladmin", "mariadb-dump", "cockroach", "clickhouse-client"}},
		{Tools: []string{"dropdb", "createdb", "dropuser", "createuser", "pg_ctl", "pg_basebackup", "mongodump", "mongorestore", "mongoimport", "mongoexport", "mysqlimport", "mysqlpump", "mysqlsh", "valkey-cli", "keydb-cli", "cqlsh", "influx", "clickhouse", "usql", "pgcli", "mycli", "litecli", "sqlplus", "isql", "bcp", "neo4j-admin", "cypher-shell", "etcdctl"}},
	}},
	{ID: "db-reset", Category: catDatabase, Description: "running migrations or resetting a database", Mode: ModeAsk, Forms: []form{
		{Tools: []string{"prisma"}, Subs: []string{"migrate reset", "migrate deploy", "migrate dev", "migrate resolve", "db push", "db execute", "db seed"}},
		{Tools: []string{"rails", "rake"}, ArgPrefixes: []string{"db:drop", "db:reset", "db:schema:load", "db:migrate", "db:rollback", "db:setup", "db:seed", "db:purge", "db:truncate_all"}},
		{Tools: []string{"sequelize", "sequelize-cli"}, ArgPrefixes: []string{"db:drop", "db:migrate", "db:seed", "db:create"}},
		{Tools: []string{"knex"}, ArgPrefixes: []string{"migrate:", "seed:run"}},
		{Tools: []string{"alembic"}, Subs: []string{"upgrade", "downgrade", "stamp"}},
		{Tools: []string{"flyway"}, Subs: []string{"migrate", "clean", "baseline", "repair", "undo"}},
		{Tools: []string{"liquibase"}, Subs: []string{"update", "drop-all", "rollback", "update-sql", "changelog-sync"}},
		{Tools: []string{"dbmate"}, Subs: []string{"up", "down", "drop", "create", "migrate", "rollback"}},
		{Tools: []string{"goose"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			for _, word := range positionals(call) {
				switch word {
				case "up", "up-by-one", "up-to", "down", "down-to", "redo", "reset":
					return "Matched: goose " + word + ".", true
				}
			}
			return "", false
		}},
		{Tools: []string{"migrate"}, Subs: []string{"up", "down", "drop", "force", "goto"}},
		{Tools: []string{"atlas"}, Subs: []string{"migrate apply", "schema apply", "schema clean"}},
		{Tools: []string{"drizzle-kit"}, Subs: []string{"push", "migrate", "drop"}},
		{Tools: []string{"typeorm"}, Subs: []string{"migration:run", "migration:revert", "schema:sync", "schema:drop"}},
		{Tools: []string{"supabase"}, Subs: []string{"db reset", "db push", "migration up", "migration repair"}},
	}},
}

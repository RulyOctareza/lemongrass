#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 The Lemongrass Authors

# Builds the UI once into a snapshot directory and launches Electron from it, with no file watching.
# A snapshot is never touched after it is created, so editing ui/src, or running this again, cannot
# change an instance that is already running from an earlier one.
set -u

KEEP=5
ui="$(cd "$(dirname "$0")/../ui" && pwd)"
snaps="$ui/.snapshot"
stage="$snaps/.staging"

mkdir -p "$snaps"
rm -rf "$stage"

# The snapshot sits inside ui/ so node_modules (node-pty is native) still resolves by walking up.
# name and version come from ui/package.json so Electron keeps the same userData directory as before.
if (cd "$ui" && npx electron-vite build --outDir "$stage/out") &&
	node -e '
		const p = require(process.argv[1])
		process.stdout.write(JSON.stringify({ name: p.name, version: p.version, main: "./out/main/index.js" }))
	' "$ui/package.json" >"$stage/package.json"; then
	target="$snaps/$(date +%Y%m%d-%H%M%S)"
	mv "$stage" "$target"
else
	rm -rf "$stage"
	target=$(ls -1d "$snaps"/[0-9]* 2>/dev/null | sort | tail -n 1)
	if [ -z "$target" ]; then
		echo "dev: the UI build failed and there is no earlier snapshot to launch." >&2
		exit 1
	fi
	echo "" >&2
	echo "==================================================================" >&2
	echo "dev: THE UI BUILD FAILED. Launching the last good snapshot instead:" >&2
	echo "     $target" >&2
	echo "     It does not contain your latest changes. Fix the error above and run make dev again." >&2
	echo "==================================================================" >&2
	echo "" >&2
fi

# Keep the newest snapshots, and never delete one a running instance was launched from.
ls -1d "$snaps"/[0-9]* | sort -r | awk -v keep="$KEEP" 'NR > keep' | while read -r old; do
	pgrep -f "$old" >/dev/null || rm -rf "$old"
done

exec "$ui/node_modules/.bin/electron" "$target"

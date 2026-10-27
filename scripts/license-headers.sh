#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Faizal Alviansyah

# Adds the license header to every tracked source file that lacks one. With --check it only lists those files and fails.
# git ls-files does not descend into the bibliothek submodule, which is a separate repository with its own license.
set -eu

LICENSE_ID="AGPL-3.0-only"
HOLDER="Faizal Alviansyah"

cd "$(dirname "$0")/.."

check=0
[ "${1:-}" = "--check" ] && check=1

year=$(date +%Y)
missing=0
IFS='
'
for file in $(git ls-files -- '*.go' '*.ts' '*.vue' '*.mjs' '*.css' '*.sh'); do
	# Only the top of the file counts, so a file that merely mentions the identifier still gets stamped.
	head -n 3 "$file" | grep -q 'SPDX-License-Identifier' && continue
	missing=$((missing + 1))
	if [ "$check" -eq 1 ]; then
		echo "$file"
		continue
	fi

	case "$file" in
	*.vue) header="<!-- SPDX-License-Identifier: $LICENSE_ID -->\\n<!-- Copyright (C) $year $HOLDER -->" ;;
	*.css) header="/* SPDX-License-Identifier: $LICENSE_ID */\\n/* Copyright (C) $year $HOLDER */" ;;
	*.sh) header="# SPDX-License-Identifier: $LICENSE_ID\\n# Copyright (C) $year $HOLDER" ;;
	*) header="// SPDX-License-Identifier: $LICENSE_ID\\n// Copyright (C) $year $HOLDER" ;;
	esac

	# The blank line after the header keeps Go from reading it as the package doc comment. A shebang must stay first.
	stamped=$(mktemp)
	awk -v header="$header" '
		NR == 1 && /^#!/ { print; print header; print ""; next }
		NR == 1 { print header; print "" }
		{ print }
		END { if (NR == 0) print header }
	' "$file" >"$stamped"
	# Writing through cat keeps the file mode, so the shell scripts stay executable.
	cat "$stamped" >"$file"
	rm -f "$stamped"
done

if [ "$check" -eq 1 ] && [ "$missing" -gt 0 ]; then
	echo "license-headers: $missing file(s) above have no license header. Run make license." >&2
	exit 1
fi
[ "$check" -eq 1 ] || echo "license-headers: stamped $missing file(s)."

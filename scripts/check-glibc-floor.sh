#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 The Lemongrass Authors

# Fails when any x86-64 ELF file under the given directories needs a newer glibc than build/rpm/glibc-floor. A native
# file takes the glibc symbol versions of the machine it was compiled on, so without this check a newer build host
# silently raises the oldest system the package starts on. Statically linked files and other architectures are skipped.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
floor=$(tr -d ' \n' < "$root/build/rpm/glibc-floor")

if [ $# -eq 0 ]; then
	echo "usage: check-glibc-floor.sh <dir>..." >&2
	exit 2
fi
for dir in "$@"; do
	if [ ! -d "$dir" ]; then
		echo "check-glibc-floor: $dir is not a directory" >&2
		exit 2
	fi
done

above() {
	[ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n 1)" = "$1" ]
}

checked=0
failed=0
files=$(find "$@" -type f)
while IFS= read -r file; do
	[ -n "$file" ] || continue
	readelf -h "$file" 2>/dev/null | grep -q 'Machine:.*X86-64' || continue
	versions=$(objdump -T "$file" 2>/dev/null | grep -o 'GLIBC_[0-9][0-9.]*' | sed 's/^GLIBC_//' | sort -uV)
	[ -n "$versions" ] || continue
	checked=$((checked + 1))
	for version in $versions; do
		above "$version" "$floor" || continue
		symbols=$(objdump -T "$file" | grep "GLIBC_$version[) ]" | awk '{print $NF}' | sort -u | tr '\n' ' ')
		echo "check-glibc-floor: $file needs GLIBC_$version, above the floor $floor: $symbols" >&2
		failed=1
	done
done <<EOF
$files
EOF

if [ "$checked" -eq 0 ]; then
	echo "check-glibc-floor: no dynamically linked x86-64 file found under $*" >&2
	exit 1
fi
if [ "$failed" -ne 0 ]; then
	exit 1
fi
echo "check-glibc-floor: $checked files need at most glibc $floor"

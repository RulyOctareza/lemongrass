#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 The Lemongrass Authors

# Installs or updates Lemongrass on macOS from the GitHub release, in one command:
#
#   curl -fsSL https://raw.githubusercontent.com/faizalv/lemongrass/main/scripts/install-mac.sh | bash
#
# The dmg is not notarized, so a dmg downloaded by a browser carries com.apple.quarantine and Gatekeeper refuses to
# open the app ("Apple could not verify Lemongrass is free of malware"). curl sets no quarantine, and the script
# clears it anyway, so the app opens on the first try.
#
# It picks the dmg for this Mac's arch from the newest stable release that has one, and the newest prerelease when no
# stable release has a dmg yet. LEMONGRASS_VERSION=v0.4.0-rc.1 installs that tag instead. The dmg is checked against
# the release's SHA256SUMS before anything is touched, and a running Lemongrass is quit before the app is replaced.
set -euo pipefail

REPO="${LEMONGRASS_REPO:-faizalv/lemongrass}"
APP="/Applications/Lemongrass.app"

say() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = Darwin ] || die "this installer is for macOS. On Linux use the rpm from https://github.com/$REPO/releases"

# A shell under Rosetta reports x86_64 on Apple silicon, so the hardware flag decides.
if [ "$(sysctl -in hw.optional.arm64 2>/dev/null)" = 1 ]; then arch=arm64; else arch=x64; fi

if [ -n "${LEMONGRASS_VERSION:-}" ]; then
	tag="$LEMONGRASS_VERSION"
	case "$tag" in v*) ;; *) tag="v$tag" ;; esac
	url="https://github.com/$REPO/releases/download/$tag/lemongrass-${tag#v}-$arch.dmg"
else
	say "Looking up the latest release of $REPO"
	# The API lists releases newest first. A tag with a '-' is a prerelease, the same rule release.yml publishes by.
	urls="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=30" |
		grep -oE "https://github.com/[^\"]+/releases/download/[^\"]+-$arch\.dmg" || true)"
	[ -n "$urls" ] || die "no release of $REPO has a macOS $arch dmg yet"
	url="$(printf '%s\n' "$urls" | grep -vE '/download/[^/]*-[^/]*/' | head -n 1 || true)"
	if [ -z "$url" ]; then
		url="$(printf '%s\n' "$urls" | head -n 1)"
		say "No stable release has a dmg yet, installing the newest prerelease"
	fi
	tag="$(printf '%s\n' "$url" | sed -E 's#.*/download/([^/]+)/.*#\1#')"
fi

dmg_name="${url##*/}"
tmp="$(mktemp -d)"
mnt="$tmp/mnt"
cleanup() {
	[ -d "$mnt" ] && hdiutil detach -quiet "$mnt" 2>/dev/null || true
	rm -rf "$tmp"
}
trap cleanup EXIT

say "Downloading $dmg_name ($tag)"
curl -fL --progress-bar -o "$tmp/$dmg_name" "$url" || die "download failed: $url"
curl -fsSL -o "$tmp/SHA256SUMS" "https://github.com/$REPO/releases/download/$tag/SHA256SUMS" ||
	die "release $tag has no SHA256SUMS, refusing to install an unchecked dmg"

say "Checking the checksum"
want="$(awk -v f="$dmg_name" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")"
[ -n "$want" ] || die "$dmg_name is not listed in SHA256SUMS"
got="$(shasum -a 256 "$tmp/$dmg_name" | awk '{ print $1 }')"
[ "$got" = "$want" ] || die "checksum mismatch for $dmg_name (got $got, want $want)"

mkdir "$mnt"
hdiutil attach -quiet -nobrowse -readonly -noautoopen -mountpoint "$mnt" "$tmp/$dmg_name"
[ -d "$mnt/Lemongrass.app" ] || die "the dmg has no Lemongrass.app"

if pgrep -xq Lemongrass; then
	say "Quitting the running Lemongrass"
	osascript -e 'tell application "Lemongrass" to quit' >/dev/null 2>&1 || true
	for _ in $(seq 1 20); do pgrep -xq Lemongrass || break; sleep 0.5; done
	pgrep -xq Lemongrass && die "Lemongrass is still running. Quit it and run the installer again."
fi

# /Applications is writable for an admin user. Anyone else is asked for a password once.
sudo=""
[ -w /Applications ] || { say "Asking for your password to write to /Applications"; sudo="sudo"; }

say "Installing to $APP"
# Copied beside the old app first, so a failed copy leaves the installed version in place.
$sudo rm -rf "$APP.new"
$sudo ditto "$mnt/Lemongrass.app" "$APP.new"
$sudo rm -rf "$APP"
$sudo mv "$APP.new" "$APP"
$sudo xattr -dr com.apple.quarantine "$APP" 2>/dev/null || true

say "Lemongrass ${tag#v} installed. Opening it."
open "$APP"

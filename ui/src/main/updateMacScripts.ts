// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { basename, dirname } from 'path'

// The macOS update replaces the running app bundle after the app has quit, so it is done by scripts the
// app writes and starts just before quitting. They are generated here, free of Electron, so their quoting
// and path rules can be unit tested.

// The executable sits at <bundle>/Contents/MacOS/<name>. Returns null when the layout does not match.
export function bundleFromExe(exe: string): string | null {
  const macos = dirname(exe)
  const contents = dirname(macos)
  const bundle = dirname(contents)
  if (basename(macos) !== 'MacOS' || basename(contents) !== 'Contents') return null
  if (!bundle.endsWith('.app')) return null
  return bundle
}

// Gatekeeper runs a quarantined app that was never moved by Finder from a read-only AppTranslocation
// copy, and an app opened inside the mounted dmg sits under /Volumes. Neither path can be replaced.
export function macUnsupportedReason(bundle: string | null, arch: string): string | null {
  if (arch !== 'arm64' && arch !== 'x64') return `Updates are not published for ${arch} Macs.`
  if (!bundle) return 'Lemongrass could not find its own app bundle, so it cannot update itself.'
  if (bundle.includes('/AppTranslocation/')) {
    return 'macOS is running this copy from a temporary location. Move Lemongrass to Applications with Finder and open it from there to get updates.'
  }
  if (bundle.startsWith('/Volumes/')) {
    return 'Lemongrass is running from the disk image. Drag it to Applications and open it from there to get updates.'
  }
  return null
}

export function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`
}

function appleScriptString(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

// Copies the staged app beside the installed one and swaps it in with mv, which stays on one volume, so a
// failed copy leaves the installed app untouched and a failed swap puts it back. Run as root, the new
// bundle takes the old one's owner, so a later update by its owner needs no password.
export function swapScript(bundle: string, staged: string): string {
  return `#!/bin/bash
set -u
bundle=${shellQuote(bundle)}
staged=${shellQuote(staged)}
[ -d "$bundle" ] && [ -d "$staged" ] || { echo "swap: missing $bundle or $staged"; exit 1; }
parent="$(dirname "$bundle")"
name="$(basename "$bundle" .app)"
next="$parent/.$name-update.app"
old="$parent/.$name-old.app"
rm -rf "$next" "$old"
if ! ditto "$staged" "$next"; then rm -rf "$next"; echo "swap: copy failed"; exit 1; fi
if [ "$(id -u)" = 0 ]; then chown -R "$(stat -f %u:%g "$bundle")" "$next"; fi
xattr -dr com.apple.quarantine "$next" 2>/dev/null
if ! mv "$bundle" "$old"; then rm -rf "$next"; echo "swap: could not move the installed app aside"; exit 1; fi
if ! mv "$next" "$bundle"; then mv "$old" "$bundle"; rm -rf "$next"; echo "swap: could not move the new app in"; exit 1; fi
rm -rf "$old"
echo "swap: done"
`
}

// osascript's administrator privileges is macOS's own password prompt, the counterpart of pkexec.
export function elevateScript(swapPath: string): string {
  return `do shell script "/bin/bash " & quoted form of ${appleScriptString(swapPath)} with prompt "Lemongrass needs your password to install the update." with administrator privileges\n`
}

export interface RunnerPaths {
  pid: number
  bundle: string
  swap: string
  elevate: string
  log: string
}

// Waits up to two minutes for the app to exit, since quitting waits on its shells. If the app is still
// running it gives up and leaves the installed version. The app is opened again whether or not the swap
// worked, so a failed update still ends with a running Lemongrass.
export function runnerScript(p: RunnerPaths): string {
  return `#!/bin/bash
set -u
exec >>${shellQuote(p.log)} 2>&1
pid=${p.pid}
bundle=${shellQuote(p.bundle)}
swap=${shellQuote(p.swap)}
elevate=${shellQuote(p.elevate)}
echo "$(date): waiting for pid $pid to exit"
for _ in $(seq 1 240); do kill -0 "$pid" 2>/dev/null || break; sleep 0.5; done
if kill -0 "$pid" 2>/dev/null; then echo "$(date): the app is still running, update skipped"; exit 1; fi
if [ -w "$(dirname "$bundle")" ] && [ -w "$bundle" ]; then
  /bin/bash "$swap" || echo "$(date): swap failed"
else
  /usr/bin/osascript "$elevate" || echo "$(date): swap as administrator failed or was cancelled"
fi
open "$bundle"
echo "$(date): opened $bundle"
`
}

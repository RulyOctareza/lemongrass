// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { win32 } from 'path'

// The installer and uninstaller find every process running from the install folder and kill it, so they start only
// once the app has quit by itself and has stopped the daemons it started. A hidden PowerShell waits for the app's pid
// and then starts the program. -Command takes inline text, which the execution policy does not restrict.
export function afterExitCommand(
  systemRoot: string,
  pid: number,
  program: string,
  args: string[]
): { file: string; args: string[] } {
  const quote = (s: string): string => `'${s.replace(/'/g, "''")}'`
  const argList = args.length ? ` -ArgumentList ${args.map(quote).join(',')}` : ''
  const script =
    `Wait-Process -Id ${pid} -Timeout 120 -ErrorAction SilentlyContinue; ` +
    `Start-Process -FilePath ${quote(program)}${argList}`
  return {
    file: win32.join(systemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe'),
    args: ['-NoProfile', '-NonInteractive', '-WindowStyle', 'Hidden', '-Command', script]
  }
}

// Must match uninstallDisplayName in electron-builder.yml, which names the uninstaller beside the app's exe.
export const WIN_UNINSTALLER = 'Uninstall Lemongrass.exe'

// Only a copy the per-user setup.exe installed can update itself: its folder holds the uninstaller, and the next
// setup.exe installs into the same place.
export function winUnsupportedReason(arch: string, hasUninstaller: boolean): string | null {
  if (arch !== 'x64') return 'Updates are published for Windows x64 only.'
  if (!hasUninstaller)
    return 'This copy was not installed from the setup, so it cannot update itself.'
  return null
}

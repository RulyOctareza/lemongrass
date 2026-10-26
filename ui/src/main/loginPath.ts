// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { execFileSync } from 'node:child_process'
import { homedir } from 'node:os'
import { delimiter, join } from 'node:path'

// An app started from the Dock or Finder gets launchd's PATH (/usr/bin:/bin:/usr/sbin:/sbin),
// so a PTY spawning `claude` or `codex` by name finds nothing and exits at once. The login
// shell's PATH is read once at startup, before any PTY or daemon is spawned. SSH_AUTH_SOCK is
// read the same way, because an ssh-agent started from a shell rc file is invisible to an app
// started from the desktop, and the vault daemon's ssh needs it to reach a key with a passphrase.

const PATH_MARKER = '__LGRASS_PATH__'
const SSH_AUTH_SOCK_MARKER = '__LGRASS_SSH_AUTH_SOCK__'
const LOGIN_SHELL_TIMEOUT_MS = 3_000

// Interactive shells may print banners around the command, so each value sits between markers.
export function extractMarked(output: string, marker: string): string | null {
  const start = output.indexOf(marker)
  if (start < 0) return null
  const end = output.indexOf(marker, start + marker.length)
  if (end < 0) return null
  const value = output.slice(start + marker.length, end).trim()
  return value.length > 0 ? value : null
}

export function extractPath(output: string): string | null {
  return extractMarked(output, PATH_MARKER)
}

// Login entries come first, then anything only the current PATH had, then the fallbacks.
export function mergePath(login: string | null, current: string, fallbacks: string[]): string {
  const dirs = [
    ...(login ?? '').split(delimiter),
    ...current.split(delimiter),
    ...fallbacks
  ].filter((dir) => dir.length > 0)
  return [...new Set(dirs)].join(delimiter)
}

function readLoginShell(): string {
  const shell = process.env.SHELL || '/bin/zsh'
  const script =
    `printf '${PATH_MARKER}%s${PATH_MARKER}' "$PATH"; ` +
    `printf '${SSH_AUTH_SOCK_MARKER}%s${SSH_AUTH_SOCK_MARKER}' "$SSH_AUTH_SOCK"`
  try {
    return execFileSync(shell, ['-ilc', script], {
      encoding: 'utf8',
      timeout: LOGIN_SHELL_TIMEOUT_MS,
      stdio: ['ignore', 'pipe', 'ignore']
    })
  } catch {
    return ''
  }
}

// A desktop session's own SSH_AUTH_SOCK wins over the login shell's.
export function pickSSHAuthSock(current: string | undefined, login: string | null): string | null {
  if (current) return current
  return login
}

export function loadLoginEnv(): void {
  if (process.platform === 'win32') return
  const output = readLoginShell()
  const home = homedir()
  const fallbacks = [join(home, '.local', 'bin'), '/opt/homebrew/bin', '/usr/local/bin']
  process.env.PATH = mergePath(extractPath(output), process.env.PATH ?? '', fallbacks)
  const sock = pickSSHAuthSock(
    process.env.SSH_AUTH_SOCK,
    extractMarked(output, SSH_AUTH_SOCK_MARKER)
  )
  if (sock) process.env.SSH_AUTH_SOCK = sock
}

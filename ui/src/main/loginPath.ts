import { execFileSync } from 'node:child_process'
import { homedir } from 'node:os'
import { delimiter, join } from 'node:path'

// An app started from the Dock or Finder gets launchd's PATH (/usr/bin:/bin:/usr/sbin:/sbin),
// so a PTY spawning `claude` or `codex` by name finds nothing and exits at once. The login
// shell's PATH is read once at startup, before any PTY is spawned.

const MARKER = '__LGRASS_PATH__'
const LOGIN_SHELL_TIMEOUT_MS = 3_000

// Interactive shells may print banners around the command, so the PATH sits between markers.
export function extractPath(output: string): string | null {
  const start = output.indexOf(MARKER)
  if (start < 0) return null
  const end = output.indexOf(MARKER, start + MARKER.length)
  if (end < 0) return null
  const path = output.slice(start + MARKER.length, end).trim()
  return path.length > 0 ? path : null
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

function readLoginPath(): string | null {
  const shell = process.env.SHELL || '/bin/zsh'
  try {
    const output = execFileSync(shell, ['-ilc', `printf '${MARKER}%s${MARKER}' "$PATH"`], {
      encoding: 'utf8',
      timeout: LOGIN_SHELL_TIMEOUT_MS,
      stdio: ['ignore', 'pipe', 'ignore']
    })
    return extractPath(output)
  } catch {
    return null
  }
}

export function loadLoginPath(): void {
  if (process.platform === 'win32') return
  const home = homedir()
  const fallbacks = [join(home, '.local', 'bin'), '/opt/homebrew/bin', '/usr/local/bin']
  process.env.PATH = mergePath(readLoginPath(), process.env.PATH ?? '', fallbacks)
}

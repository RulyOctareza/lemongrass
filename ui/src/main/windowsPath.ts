// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { execFile, execFileSync } from 'node:child_process'
import { homedir } from 'node:os'
import { join } from 'node:path'
import { mergeWindowsPath, parseRegValue, type Env } from './winExec'

// Windows has no login shell to read PATH from: the PATH a new terminal gets lives in the registry, and a running
// process never sees an entry added after it started, such as lgrassconf install's bin dir or a winget install.
// The app reads it once at start, and every tab spawn reads it again, the way Windows Terminal builds a new tab's
// environment.

const MACHINE_ENV_KEY = 'HKLM\\SYSTEM\\CurrentControlSet\\Control\\Session Manager\\Environment'
const USER_ENV_KEY = 'HKCU\\Environment'
const REG_TIMEOUT_MS = 3_000

function queryRegPath(key: string): Promise<string | undefined> {
  return new Promise((resolve) => {
    execFile(
      'reg.exe',
      ['query', key, '/v', 'Path'],
      { encoding: 'utf8', timeout: REG_TIMEOUT_MS, windowsHide: true },
      (failure, stdout) => resolve(failure ? undefined : parseRegValue(stdout, 'Path'))
    )
  })
}

function queryRegPathSync(key: string): string | undefined {
  try {
    const out = execFileSync('reg.exe', ['query', key, '/v', 'Path'], {
      encoding: 'utf8',
      timeout: REG_TIMEOUT_MS,
      windowsHide: true,
      stdio: ['ignore', 'pipe', 'ignore']
    })
    return parseRegValue(out, 'Path')
  } catch {
    return undefined
  }
}

// lgrass's and Claude's native installer's bin dir and npm's global bin, kept in case the registry cannot be read.
function fallbacks(env: Env): string {
  const dirs = [join(env.USERPROFILE ?? homedir(), '.local', 'bin')]
  if (env.APPDATA) dirs.push(join(env.APPDATA, 'npm'))
  return dirs.join(';')
}

function merged(machine: string | undefined, user: string | undefined): string {
  const current = `${process.env.PATH ?? ''};${fallbacks(process.env)}`
  return mergeWindowsPath(machine, user, current, process.env)
}

// The PATH a new tab gets, from the registry as it is now.
export async function freshWindowsPath(): Promise<string> {
  const [machine, user] = await Promise.all([
    queryRegPath(MACHINE_ENV_KEY),
    queryRegPath(USER_ENV_KEY)
  ])
  return merged(machine, user)
}

// Runs once at start, before any daemon or git call, so every child of the app starts with the current PATH.
export function loadWindowsPath(): void {
  process.env.PATH = merged(queryRegPathSync(MACHINE_ENV_KEY), queryRegPathSync(USER_ENV_KEY))
}

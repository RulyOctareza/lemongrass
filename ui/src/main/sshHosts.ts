// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { execFile } from 'node:child_process'
import { readdirSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { basename, dirname, isAbsolute, join } from 'node:path'
import type { ResolvedSSHHost } from '../preload/types'

// The Connections form lists the human's own ssh config hosts to pick from, and the vault daemon
// later runs `ssh -W` against the picked alias (dbgate/sshdial.go). Both run as the same user, so
// they read the same config.

const MAX_INCLUDE_DEPTH = 16
const RESOLVE_TIMEOUT_MS = 3_000

// Mirrors sshAliasPattern in dbgate/sshdial.go, which rejects anything else before ssh runs.
const ALIAS_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]*$/

export interface ConfigFiles {
  read(path: string): string | null
  list(dir: string): string[]
}

// Splits a config line's arguments on whitespace, keeping double-quoted runs together.
function splitArgs(rest: string): string[] {
  const args: string[] = []
  const re = /"([^"]*)"|(\S+)/g
  let m: RegExpExecArray | null
  while ((m = re.exec(rest)) !== null) args.push(m[1] ?? m[2])
  return args
}

function globToRegExp(glob: string): RegExp {
  const body = glob
    .replace(/[.+^${}()|[\]\\]/g, '\\$&')
    .replace(/\*/g, '.*')
    .replace(/\?/g, '.')
  return new RegExp(`^${body}$`)
}

// Include paths are relative to the user's ssh directory and may glob in their last segment.
function includePaths(arg: string, sshDir: string, home: string, files: ConfigFiles): string[] {
  let path = arg.startsWith('~/') ? join(home, arg.slice(2)) : arg
  if (!isAbsolute(path)) path = join(sshDir, path)
  const name = basename(path)
  if (!/[*?]/.test(name)) return [path]
  const re = globToRegExp(name)
  return files
    .list(dirname(path))
    .filter((entry) => re.test(entry))
    .sort()
    .map((entry) => join(dirname(path), entry))
}

// Lists the concrete Host entries of an ssh config in file order, following Include. A pattern
// (`*`, `?` or a `!` negation) names no single host, so it is not offered as a pick.
export function parseSSHConfigHosts(
  path: string,
  home: string,
  files: ConfigFiles,
  depth = 0,
  seen = new Set<string>()
): string[] {
  if (depth > MAX_INCLUDE_DEPTH || seen.has(path)) return []
  seen.add(path)
  const text = files.read(path)
  if (text === null) return []

  const sshDir = join(home, '.ssh')
  const hosts: string[] = []
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) continue
    const m = /^(\S+?)(?:\s*=\s*|\s+)(.*)$/.exec(line)
    if (!m) continue
    const keyword = m[1].toLowerCase()
    const args = splitArgs(m[2])
    if (keyword === 'host') {
      for (const pattern of args) {
        if (!/[*?!]/.test(pattern)) hosts.push(pattern)
      }
    } else if (keyword === 'include') {
      for (const arg of args) {
        for (const included of includePaths(arg, sshDir, home, files)) {
          hosts.push(...parseSSHConfigHosts(included, home, files, depth + 1, seen))
        }
      }
    }
  }
  return [...new Set(hosts)]
}

// Reads `ssh -G` output, OpenSSH's fully resolved config for one host.
export function parseResolvedHost(output: string): ResolvedSSHHost {
  const values = new Map<string, string>()
  for (const line of output.split(/\r?\n/)) {
    const m = /^(\S+)\s+(.*)$/.exec(line.trim())
    if (m && !values.has(m[1].toLowerCase())) values.set(m[1].toLowerCase(), m[2])
  }
  const proxyJump = values.get('proxyjump') ?? ''
  return {
    hostname: values.get('hostname') ?? '',
    user: values.get('user') ?? '',
    port: values.get('port') ?? '',
    proxyJump: proxyJump === 'none' ? '' : proxyJump
  }
}

const diskFiles: ConfigFiles = {
  read(path) {
    try {
      return readFileSync(path, 'utf8')
    } catch {
      return null
    }
  },
  list(dir) {
    try {
      return readdirSync(dir)
    } catch {
      return []
    }
  }
}

export function listSSHHosts(): string[] {
  const home = homedir()
  return parseSSHConfigHosts(join(home, '.ssh', 'config'), home, diskFiles)
}

// `ssh -G` only prints the resolved config; it opens no connection.
export function resolveSSHHost(alias: string): Promise<ResolvedSSHHost> {
  if (!ALIAS_PATTERN.test(alias)) {
    return Promise.reject(new Error(`"${alias}" is not a valid ssh host alias.`))
  }
  return new Promise((resolve, reject) => {
    execFile(
      'ssh',
      ['-G', '--', alias],
      { timeout: RESOLVE_TIMEOUT_MS, encoding: 'utf8' },
      (err, stdout, stderr) => {
        if (err) {
          reject(new Error(stderr.trim() || err.message))
          return
        }
        resolve(parseResolvedHost(stdout))
      }
    )
  })
}

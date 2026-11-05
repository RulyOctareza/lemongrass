// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { readFileSync, statSync } from 'node:fs'
import { win32 } from 'node:path'

// Windows-only lookups for spawning a program by name, written with path.win32 and with the file reads passed in,
// so the tests run on any host.

export type Env = Record<string, string | undefined>

// Windows environment names are case-insensitive, but a copied env object is a plain object.
export function envValue(env: Env, name: string): string | undefined {
  const key = Object.keys(env).find((k) => k.toLowerCase() === name.toLowerCase())
  return key === undefined ? undefined : env[key]
}

const DEFAULT_PATHEXT = '.COM;.EXE;.BAT;.CMD'

export interface FileReader {
  isFile: (path: string) => boolean
  read: (path: string) => string
}

const disk: FileReader = {
  isFile: (path) => {
    try {
      return statSync(path).isFile()
    } catch {
      return false
    }
  },
  read: (path) => readFileSync(path, 'utf8')
}

// The full path of name the way cmd.exe finds it: each PATH directory in order, each PATHEXT extension in order,
// unless name already carries an extension.
export function findOnPath(name: string, env: Env, fs: FileReader = disk): string | undefined {
  const exts = (envValue(env, 'PATHEXT') || DEFAULT_PATHEXT).split(';').filter(Boolean)
  const candidates = win32.extname(name) ? [name] : exts.map((ext) => name + ext.toLowerCase())
  for (const dir of (envValue(env, 'PATH') ?? '').split(';')) {
    if (!dir) continue
    for (const candidate of candidates) {
      const full = win32.join(dir.replace(/^"|"$/g, ''), candidate)
      if (fs.isFile(full)) return full
    }
  }
  return undefined
}

// What an npm, pnpm or yarn .cmd shim runs: the quoted target beside %dp0% on the line that passes %*.
export function shimTarget(shimPath: string, text: string): string | undefined {
  const dir = win32.dirname(shimPath) + '\\'
  for (const line of text.split(/\r?\n/).reverse()) {
    if (!line.includes('%*')) continue
    const quoted = [...line.matchAll(/"([^"]+)"/g)].map((m) => m[1])
    const target = quoted.find((q) => /%~?dp0%?/i.test(q) && !/node(\.exe)?$/i.test(q))
    if (target) return win32.normalize(target.replace(/%~?dp0%?/gi, dir))
  }
  return undefined
}

export type Launch = { file: string; args: string[] } | { error: string }

// A .cmd or .bat is never run through cmd.exe: cmd re-parses the arguments, which mangles the JSON and free text a
// tab passes, and turns Ctrl+C into a "Terminate batch job" prompt. The shim's own target is run instead.
export function windowsLaunch(
  command: string,
  args: string[],
  env: Env,
  fs: FileReader = disk
): Launch {
  const found = findOnPath(command, env, fs)
  if (!found) {
    return {
      error: `${command} was not found on PATH. Install the ${command} CLI, then open a new tab.`
    }
  }
  const ext = win32.extname(found).toLowerCase()
  if (ext !== '.cmd' && ext !== '.bat') return { file: found, args }
  const unreadable = {
    error: `Lemongrass could not read the launcher ${found}. Reinstall the ${command} CLI, then open a new tab.`
  }
  let target: string | undefined
  try {
    target = shimTarget(found, fs.read(found))
  } catch {
    return unreadable
  }
  if (!target || !fs.isFile(target)) return unreadable
  if (!/\.(c|m)?js$/i.test(target)) return { file: target, args }
  const besideShim = win32.join(win32.dirname(found), 'node.exe')
  const node = fs.isFile(besideShim) ? besideShim : findOnPath('node', env, fs)
  if (!node)
    return {
      error: `node was not found on PATH, and ${command} needs it. Install Node.js, then open a new tab.`
    }
  return { file: node, args: [target, ...args] }
}

// The Shell entry on Windows: PowerShell 7, then Windows PowerShell, then the command interpreter.
export function windowsShell(env: Env, fs: FileReader = disk): string {
  return (
    findOnPath('pwsh.exe', env, fs) ??
    findOnPath('powershell.exe', env, fs) ??
    envValue(env, 'ComSpec') ??
    'cmd.exe'
  )
}

// One reg.exe query result: the value's data, from a line such as "    Path    REG_EXPAND_SZ    C:\...".
export function parseRegValue(output: string, name: string): string | undefined {
  for (const line of output.split(/\r?\n/)) {
    const match = /^\s+(\S+)\s+REG_(?:EXPAND_)?SZ\s+(.*)$/.exec(line)
    if (match && match[1].toLowerCase() === name.toLowerCase()) return match[2].trim()
  }
  return undefined
}

// Expands %NAME% from env; an unknown name stays as written, the way Windows leaves it.
export function expandEnv(value: string, env: Env): string {
  return value.replace(/%([^%;]+)%/g, (whole, name: string) => envValue(env, name) ?? whole)
}

// Machine entries, then user entries, then anything only the app's own PATH had, without repeats.
export function mergeWindowsPath(
  machine: string | undefined,
  user: string | undefined,
  current: string,
  env: Env
): string {
  const dirs = [machine ?? '', user ?? '', current]
    .flatMap((part) => expandEnv(part, env).split(';'))
    .filter((dir) => dir.length > 0)
  const seen = new Set<string>()
  return dirs
    .filter((dir) => {
      const key = dir.replace(/\\+$/, '').toLowerCase()
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
    .join(';')
}

// Sets PATH in a copied env object, replacing whatever casing the key had, so the child never gets two of them.
export function withPath(env: Env, value: string): Env {
  const out: Env = {}
  for (const [k, v] of Object.entries(env)) if (k.toLowerCase() !== 'path') out[k] = v
  out.Path = value
  return out
}

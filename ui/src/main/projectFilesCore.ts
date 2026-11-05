// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import {
  closeSync,
  cpSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  readSync,
  realpathSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
  type Stats
} from 'fs'
import { lstat, opendir, readdir, stat } from 'fs/promises'
import { basename, dirname, extname, join, relative, resolve, sep } from 'path'
import type { RunResult } from './gitRun'
import type {
  ProjectEntry,
  ProjectFileResult,
  ProjectListResult,
  ProjectOpError,
  ProjectOpResult,
  ProjectOpsResult,
  ProjectSearchResult
} from '../preload/types'

// Kept free of electron and sibling value imports so node --test can load it straight from
// source; the IPC, shell and watcher side lives in projectFiles.ts.

// Runs read-only git in the project root; input goes to stdin.
export type ProjectGit = (args: string[], input?: string) => Promise<RunResult>

// Never listed, searched, read or touched: .git is git's own, biblio has its own view.
// Lowercased, so a case-insensitive filesystem cannot reach them through another spelling.
const HIDDEN_ROOT_NAMES = new Set(['.git', 'biblio'])

export const MAX_TEXT_BYTES = 2 * 1024 * 1024
const BINARY_SNIFF_BYTES = 8 * 1024
export const IMAGE_MIME: Record<string, string> = {
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp'
}
export const SEARCH_MAX_MATCHES = 2000
export const SEARCH_MAX_WALK = 100_000

export class OpError extends Error {
  readonly code: ProjectOpError

  constructor(code: ProjectOpError) {
    super(code)
    this.code = code
  }
}

export function toRelative(root: string, absolute: string): string {
  return relative(root, absolute).split(sep).join('/')
}

function isHiddenTop(root: string, absolute: string): boolean {
  const first = relative(root, absolute).split(sep)[0]
  return HIDDEN_ROOT_NAMES.has(first.toLowerCase())
}

// True when absolute is root itself or under it, and not under a hidden top-level folder.
export function isConfined(root: string, absolute: string): boolean {
  if (absolute !== root && !absolute.startsWith(root + sep)) return false
  return absolute === root || !isHiddenTop(root, absolute)
}

// Lexical check of a renderer path against the real root. Paths are '/' separated and relative.
export function resolveLexical(root: string, relPath: string): string | null {
  if (typeof relPath !== 'string' || relPath.includes('\0')) return null
  if (relPath.startsWith('/') || /^[a-zA-Z]:/.test(relPath)) return null
  const target = resolve(root, ...relPath.split('/').filter(Boolean))
  return isConfined(root, target) ? target : null
}

// For reading or listing: the path with every symlink followed must still be confined, so a
// link inside the project that points outside it shows in the tree but cannot be opened.
export function resolveReal(root: string, relPath: string): string | null {
  const lexical = resolveLexical(root, relPath)
  if (!lexical) return null
  let real: string
  try {
    real = realpathSync(lexical)
  } catch {
    return null
  }
  return isConfined(root, real) ? real : null
}

// For operating on an entry itself (rename, move, trash a symlink rather than its target): only
// the parent folder is followed, so the entry is addressed inside the real root.
export function resolveEntry(root: string, relPath: string): string | null {
  const lexical = resolveLexical(root, relPath)
  if (!lexical || lexical === root) return null
  const parent = resolveReal(root, toRelative(root, dirname(lexical)))
  if (!parent) return null
  const target = join(parent, basename(lexical))
  return isConfined(root, target) && target !== root ? target : null
}

const WINDOWS_RESERVED = /^(con|prn|aux|nul|com[0-9]|lpt[0-9])(\..*)?$/i

// Names Windows refuses or silently changes: its reserved characters, a trailing dot or space, which it strips, and
// device names such as NUL, with or without an extension.
export function isWindowsInvalidName(name: string): boolean {
  return (
    /[<>:"|?*]/.test(name) ||
    [...name].some((ch) => ch.charCodeAt(0) < 32) ||
    /[. ]$/.test(name) ||
    WINDOWS_RESERVED.test(name)
  )
}

export function isValidName(name: string, atRoot: boolean): boolean {
  if (typeof name !== 'string' || name.trim() === '' || name === '.' || name === '..') return false
  if (name.includes('/') || name.includes('\0') || (sep === '\\' && name.includes('\\')))
    return false
  if (sep === '\\' && isWindowsInvalidName(name)) return false
  return !(atRoot && HIDDEN_ROOT_NAMES.has(name.toLowerCase()))
}

// "name copy.ext", then "name copy 2.ext". extname gives a dotfile such as .env no extension.
export function copyName(name: string, taken: (candidate: string) => boolean): string {
  const extension = extname(name)
  const stem = name.slice(0, name.length - extension.length)
  let candidate = `${stem} copy${extension}`
  for (let n = 2; taken(candidate); n += 1) candidate = `${stem} copy ${n}${extension}`
  return candidate
}

// Coarse filter only: every query character appears in order, case-insensitively, whitespace
// ignored. Ranking and highlighting are the renderer's shared matcher.
export function isSubsequence(query: string, path: string): boolean {
  const needle = query.toLowerCase().replace(/\s+/g, '')
  const haystack = path.toLowerCase()
  let at = 0
  for (const char of needle) {
    at = haystack.indexOf(char, at)
    if (at === -1) return false
    at += 1
  }
  return true
}

function exists(path: string): boolean {
  try {
    lstatSync(path)
    return true
  } catch {
    return false
  }
}

function errorCode(error: unknown): ProjectOpError {
  if (error instanceof OpError) return error.code
  const code = (error as NodeJS.ErrnoException)?.code
  if (code === 'EEXIST' || code === 'ENOTEMPTY') return 'exists'
  if (code === 'ENOENT') return 'notFound'
  return 'failed'
}

async function ignoredSet(git: ProjectGit, relPaths: string[]): Promise<Set<string>> {
  if (relPaths.length === 0) return new Set()
  const result = await git(['check-ignore', '-z', '--stdin'], relPaths.join('\0') + '\0')
  // Exit 1 means nothing matched, 128 means not a repository; both leave every entry unignored.
  if (result.code !== 0) return new Set()
  return new Set(result.stdout.split('\0').filter(Boolean))
}

export async function listDir(
  root: string,
  relDir: string,
  git: ProjectGit
): Promise<ProjectListResult> {
  const dir = resolveReal(root, relDir)
  if (!dir) return { error: resolveLexical(root, relDir) ? 'notFound' : 'outside' }
  try {
    const names = await readdir(dir)
    const entries: ProjectEntry[] = []
    for (const name of names) {
      const full = join(dir, name)
      if (!isConfined(root, full)) continue
      let info: Stats
      try {
        info = await lstat(full)
      } catch {
        continue
      }
      const symlink = info.isSymbolicLink()
      if (symlink) {
        try {
          info = await stat(full)
        } catch {
          // A broken link keeps its own lstat and shows as a file.
        }
      }
      const isDir = info.isDirectory()
      entries.push({
        name,
        path: toRelative(root, full),
        type: isDir ? 'dir' : 'file',
        ...(symlink ? { symlink: true as const } : {}),
        ignored: false,
        ...(isDir ? {} : { sizeBytes: info.size }),
        modifiedMs: info.mtimeMs
      })
    }
    const ignored = await ignoredSet(
      git,
      entries.map((e) => e.path)
    )
    for (const entry of entries) entry.ignored = ignored.has(entry.path)
    entries.sort((a, b) => {
      if (a.type !== b.type) return a.type === 'dir' ? -1 : 1
      return a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' })
    })
    return { entries }
  } catch (error) {
    return { error: errorCode(error) === 'notFound' ? 'notFound' : 'failed' }
  }
}

export function readProjectFile(root: string, relPath: string): ProjectFileResult | null {
  const target = resolveReal(root, relPath)
  if (!target) return null
  try {
    const info = statSync(target)
    if (!info.isFile()) return null
    if (IMAGE_MIME[extname(target).toLowerCase()])
      return { kind: 'image', modifiedMs: info.mtimeMs }
    const head = Buffer.alloc(Math.min(info.size, BINARY_SNIFF_BYTES))
    const fd = openSync(target, 'r')
    try {
      readSync(fd, head, 0, head.length, 0)
    } finally {
      closeSync(fd)
    }
    if (head.includes(0)) return { kind: 'binary' }
    try {
      new TextDecoder('utf-8', { fatal: true }).decode(head, { stream: true })
    } catch {
      return { kind: 'binary' }
    }
    if (info.size > MAX_TEXT_BYTES) return { kind: 'tooLarge' }
    return { kind: 'text', text: readFileSync(target, 'utf-8') }
  } catch {
    return null
  }
}

// The absolute path of an image the project-file:// scheme may serve, or null.
export function imagePath(root: string, relPath: string): string | null {
  const target = resolveReal(root, relPath)
  if (!target || !IMAGE_MIME[extname(target).toLowerCase()]) return null
  try {
    return statSync(target).isFile() ? target : null
  } catch {
    return null
  }
}

interface SearchCollector {
  paths: ProjectSearchResult['paths']
  truncated: boolean
}

function collect(
  out: SearchCollector,
  query: string,
  path: string,
  type: 'dir' | 'file',
  ignored = false
): boolean {
  if (!isSubsequence(query, path)) return true
  if (out.paths.length >= SEARCH_MAX_MATCHES) {
    out.truncated = true
    return false
  }
  out.paths.push(ignored ? { path, type, ignored: true } : { path, type })
  return true
}

async function walk(
  root: string,
  start: string,
  query: string,
  out: SearchCollector,
  budget: { left: number },
  ignored = false
): Promise<void> {
  const queue = [start]
  while (queue.length > 0) {
    const dir = queue.shift()!
    let handle
    try {
      handle = await opendir(dir)
    } catch {
      continue
    }
    for await (const entry of handle) {
      if (budget.left <= 0) {
        out.truncated = true
        return
      }
      budget.left -= 1
      const full = join(dir, entry.name)
      if (!isConfined(root, full)) continue
      const isDir = entry.isDirectory()
      if (!collect(out, query, toRelative(root, full), isDir ? 'dir' : 'file', ignored)) return
      if (isDir) queue.push(full)
    }
  }
}

function splitZ(raw: string): string[] {
  return raw.split('\0').filter(Boolean)
}

// Lists through git when the project is a repository, which skips gitignored folders for free;
// a gitignored folder is walked only when a query segment names it, such as node_modules/react.
export async function searchProject(
  root: string,
  query: string,
  git: ProjectGit
): Promise<ProjectSearchResult> {
  const out: SearchCollector = { paths: [], truncated: false }
  if (query.replace(/\s+/g, '') === '') return out
  const [cached, others, deleted] = await Promise.all([
    git(['ls-files', '-z', '-s', '--cached']),
    git(['ls-files', '-z', '--others', '--exclude-standard']),
    git(['ls-files', '-z', '--deleted'])
  ])
  if (cached.code !== 0 || others.code !== 0) {
    await walk(root, root, query, out, { left: SEARCH_MAX_WALK })
    return out
  }
  const gone = new Set(deleted.code === 0 ? splitZ(deleted.stdout) : [])
  const files = new Map<string, 'dir' | 'file'>()
  for (const line of splitZ(cached.stdout)) {
    const tab = line.indexOf('\t')
    const path = line.slice(tab + 1)
    // Mode 160000 is a submodule, a folder on disk.
    if (!gone.has(path)) files.set(path, line.startsWith('160000 ') ? 'dir' : 'file')
  }
  for (const path of splitZ(others.stdout)) files.set(path, 'file')
  const dirs = new Set<string>()
  for (const path of files.keys()) {
    for (let at = path.indexOf('/'); at !== -1; at = path.indexOf('/', at + 1))
      dirs.add(path.slice(0, at))
  }
  const isHiddenRel = (path: string): boolean =>
    HIDDEN_ROOT_NAMES.has(path.split('/')[0].toLowerCase())
  for (const dir of dirs) {
    if (!isHiddenRel(dir) && !collect(out, query, dir, 'dir')) return out
  }
  for (const [path, type] of files) {
    if (!isHiddenRel(path) && !collect(out, query, path, type)) return out
  }

  const named = new Set(
    query
      .toLowerCase()
      .split('/')
      .map((s) => s.trim())
      .filter(Boolean)
  )
  if (named.size === 0) return out
  const ignored = await git([
    'ls-files',
    '-z',
    '--others',
    '--ignored',
    '--exclude-standard',
    '--directory'
  ])
  if (ignored.code !== 0) return out
  const budget = { left: SEARCH_MAX_WALK }
  for (const entry of splitZ(ignored.stdout)) {
    if (!entry.endsWith('/')) continue
    const rel = entry.slice(0, -1)
    if (isHiddenRel(rel) || !named.has(rel.split('/').pop()!.toLowerCase())) continue
    const full = resolveReal(root, rel)
    if (!full) continue
    if (!collect(out, query, rel, 'dir', true)) return out
    await walk(root, full, query, out, budget, true)
    if (out.truncated) return out
  }
  return out
}

function parentDir(root: string, relDir: string): string {
  const dir = resolveReal(root, relDir)
  if (!dir) throw new OpError(resolveLexical(root, relDir) ? 'notFound' : 'outside')
  if (!statSync(dir).isDirectory()) throw new OpError('notFound')
  return dir
}

function entry(root: string, relPath: string): string {
  const target = resolveEntry(root, relPath)
  if (!target) throw new OpError(resolveLexical(root, relPath) ? 'notFound' : 'outside')
  if (!exists(target)) throw new OpError('notFound')
  return target
}

function checkedName(dir: string, root: string, name: string): string {
  if (!isValidName(name, dir === root)) throw new OpError('invalidName')
  return join(dir, name)
}

function single(root: string, action: () => string): ProjectOpResult {
  try {
    return { ok: true, path: toRelative(root, action()) }
  } catch (error) {
    return { error: errorCode(error) }
  }
}

function many(root: string, paths: string[], action: (path: string) => string): ProjectOpsResult {
  const done: string[] = []
  for (const path of paths) {
    try {
      done.push(toRelative(root, action(path)))
    } catch (error) {
      return { error: errorCode(error), done }
    }
  }
  return { ok: true, paths: done }
}

export function createFile(root: string, relDir: string, name: string): ProjectOpResult {
  return single(root, () => {
    const target = checkedName(parentDir(root, relDir), root, name)
    writeFileSync(target, '', { flag: 'wx' })
    return target
  })
}

export function createFolder(root: string, relDir: string, name: string): ProjectOpResult {
  return single(root, () => {
    const target = checkedName(parentDir(root, relDir), root, name)
    mkdirSync(target)
    return target
  })
}

function sameEntry(a: string, b: string): boolean {
  try {
    const left = lstatSync(a)
    const right = lstatSync(b)
    return left.ino === right.ino && left.dev === right.dev
  } catch {
    return false
  }
}

export function renameEntry(root: string, relPath: string, newName: string): ProjectOpResult {
  return single(root, () => {
    const source = entry(root, relPath)
    const target = checkedName(dirname(source), root, newName)
    if (target === source) return source
    // A case-only rename on a case-insensitive filesystem finds the source itself at the target.
    if (exists(target) && !sameEntry(source, target)) throw new OpError('exists')
    renameSync(source, target)
    return target
  })
}

function isInsideOrSame(parent: string, child: string): boolean {
  return child === parent || child.startsWith(parent + sep)
}

export function moveEntries(root: string, relPaths: string[], relDest: string): ProjectOpsResult {
  let dest: string
  try {
    dest = parentDir(root, relDest)
  } catch (error) {
    return { error: errorCode(error), done: [] }
  }
  return many(root, relPaths, (relPath) => {
    const source = entry(root, relPath)
    const target = checkedName(dest, root, basename(source))
    if (target === source) return source
    if (isInsideOrSame(source, dest)) throw new OpError('intoItself')
    if (exists(target)) throw new OpError('exists')
    try {
      renameSync(source, target)
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'EXDEV') throw error
      // A folder that is a mount point cannot be renamed across it; copy, then remove.
      cpSync(source, target, {
        recursive: true,
        errorOnExist: true,
        force: false,
        verbatimSymlinks: true
      })
      rmSync(source, { recursive: true })
    }
    return target
  })
}

function copyInto(root: string, source: string, dest: string): string {
  if (isInsideOrSame(source, dest)) throw new OpError('intoItself')
  let name = basename(source)
  if (exists(join(dest, name))) name = copyName(name, (candidate) => exists(join(dest, candidate)))
  const target = checkedName(dest, root, name)
  cpSync(source, target, {
    recursive: true,
    errorOnExist: true,
    force: false,
    verbatimSymlinks: true
  })
  return target
}

export function copyEntries(root: string, relPaths: string[], relDest: string): ProjectOpsResult {
  let dest: string
  try {
    dest = parentDir(root, relDest)
  } catch (error) {
    return { error: errorCode(error), done: [] }
  }
  return many(root, relPaths, (relPath) => copyInto(root, entry(root, relPath), dest))
}

export function duplicateEntry(root: string, relPath: string): ProjectOpResult {
  return single(root, () => {
    const source = entry(root, relPath)
    const name = copyName(basename(source), (candidate) => exists(join(dirname(source), candidate)))
    const target = checkedName(dirname(source), root, name)
    cpSync(source, target, {
      recursive: true,
      errorOnExist: true,
      force: false,
      verbatimSymlinks: true
    })
    return target
  })
}

// Resolves each path for an operation the caller performs itself, such as the trash.
export function resolveEntries(
  root: string,
  relPaths: string[]
): { targets: string[] } | { error: ProjectOpError } {
  const targets: string[] = []
  for (const relPath of relPaths) {
    try {
      targets.push(entry(root, relPath))
    } catch (error) {
      return { error: errorCode(error) }
    }
  }
  return { targets }
}

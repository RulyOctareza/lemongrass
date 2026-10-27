// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type {
  GitDiff,
  GitDiffHunk,
  GitDiffLine,
  GitDiffRow,
  GitFile,
  GitFileGroup,
  GitFileKind,
  GitBlameLine,
  GitCommit,
  GitRef,
  GitRevFile
} from '../preload/types'

export interface ParsedStatus {
  branch: string | null
  detached: boolean
  upstream: string | null
  ahead: number
  behind: number
  hasCommits: boolean
  files: GitFile[]
}

function kindOf(xy: string): GitFileKind {
  const [index, worktree] = [xy[0], xy[1]]
  if (index === 'D' || worktree === 'D') return 'deleted'
  if (index === 'A') return 'added'
  if (index === 'R' || worktree === 'R') return 'renamed'
  if (index === 'C' || worktree === 'C') return 'copied'
  if (index === 'T' || worktree === 'T') return 'typechange'
  return 'modified'
}

function isStaged(xy: string): boolean {
  return xy[0] !== '.' && xy[0] !== '?'
}

export function groupOf(kind: GitFileKind, xy: string): GitFileGroup {
  if (kind === 'conflicted') return 'conflicted'
  if (kind === 'untracked') return 'unstagedNew'
  if (kind === 'deleted') return 'deleted'
  if (kind === 'renamed' || kind === 'copied') return 'renamed'
  if (xy[0] === 'A') return 'stagedNew'
  return 'changed'
}

export function parseStatus(raw: string): ParsedStatus {
  const result: ParsedStatus = {
    branch: null,
    detached: false,
    upstream: null,
    ahead: 0,
    behind: 0,
    hasCommits: true,
    files: []
  }
  const records = raw.split('\0')
  for (let i = 0; i < records.length; i++) {
    const record = records[i]
    if (!record) continue
    if (record.startsWith('# ')) {
      const [key, ...rest] = record.slice(2).split(' ')
      const value = rest.join(' ')
      if (key === 'branch.oid') result.hasCommits = value !== '(initial)'
      else if (key === 'branch.head') {
        result.detached = value === '(detached)'
        result.branch = result.detached ? null : value
      } else if (key === 'branch.upstream') result.upstream = value
      else if (key === 'branch.ab') {
        const match = /^\+(\d+) -(\d+)$/.exec(value)
        if (match) {
          result.ahead = Number(match[1])
          result.behind = Number(match[2])
        }
      }
      continue
    }
    const type = record[0]
    if (type === '?') {
      result.files.push({
        path: record.slice(2),
        kind: 'untracked',
        group: 'unstagedNew',
        staged: false
      })
    } else if (type === '1') {
      const fields = record.split(' ')
      const kind = kindOf(fields[1])
      result.files.push({
        path: fields.slice(8).join(' '),
        kind,
        group: groupOf(kind, fields[1]),
        staged: isStaged(fields[1])
      })
    } else if (type === '2') {
      const fields = record.split(' ')
      const origPath = records[++i]
      const kind = kindOf(fields[1])
      result.files.push({
        path: fields.slice(9).join(' '),
        origPath,
        kind,
        group: groupOf(kind, fields[1]),
        staged: isStaged(fields[1])
      })
    } else if (type === 'u') {
      const fields = record.split(' ')
      result.files.push({
        path: fields.slice(10).join(' '),
        kind: 'conflicted',
        group: 'conflicted',
        staged: true
      })
    }
  }
  result.files.sort((a, b) => a.path.localeCompare(b.path))
  return result
}

const NAME_STATUS_KINDS: Record<string, GitFileKind> = {
  A: 'added',
  M: 'modified',
  D: 'deleted',
  R: 'renamed',
  C: 'copied',
  T: 'typechange'
}

// Parses `git diff --name-status -z`: a status token, then one path, or two for a rename or copy.
export function parseNameStatus(raw: string): GitRevFile[] {
  const tokens = raw.split('\0')
  const files: GitRevFile[] = []
  for (let i = 0; i < tokens.length; i++) {
    const status = tokens[i]
    if (!status) continue
    const kind = NAME_STATUS_KINDS[status[0]] ?? 'modified'
    if (status[0] === 'R' || status[0] === 'C') {
      files.push({ path: tokens[i + 2], origPath: tokens[i + 1], kind })
      i += 2
    } else {
      files.push({ path: tokens[++i], kind })
    }
  }
  return files.sort((a, b) => a.path.localeCompare(b.path))
}

const HUNK_HEADER = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/

function stripCarriageReturn(text: string): string {
  return text.endsWith('\r') ? text.slice(0, -1) : text
}

export function parseDiff(raw: string): GitDiff {
  if (!raw.trim()) return { status: 'empty' }
  const lines = raw.split('\n')
  if (
    lines.some((line) => line.startsWith('Binary files ') || line.startsWith('GIT binary patch'))
  ) {
    return { status: 'binary' }
  }

  const hunks: GitDiffHunk[] = []
  let current: GitDiffHunk | null = null
  let oldNumber = 0
  let newNumber = 0
  let removals: GitDiffLine[] = []
  let additions: GitDiffLine[] = []
  let added = 0
  let removed = 0

  const flush = (): void => {
    if (!current) return
    const count = Math.max(removals.length, additions.length)
    for (let i = 0; i < count; i++) {
      const row: GitDiffRow = { left: removals[i] ?? null, right: additions[i] ?? null }
      current.rows.push(row)
    }
    removals = []
    additions = []
  }

  for (const line of lines) {
    const header = HUNK_HEADER.exec(line)
    if (header) {
      flush()
      current = { header: line, rows: [] }
      hunks.push(current)
      oldNumber = Number(header[1])
      newNumber = Number(header[2])
      continue
    }
    if (!current || line.length === 0 || line.startsWith('\\')) continue
    const marker = line[0]
    const text = stripCarriageReturn(line.slice(1))
    if (marker === '-') {
      removals.push({ number: oldNumber++, text, type: 'del' })
      removed++
    } else if (marker === '+') {
      additions.push({ number: newNumber++, text, type: 'add' })
      added++
    } else if (marker === ' ') {
      flush()
      current.rows.push({
        left: { number: oldNumber++, text, type: 'context' },
        right: { number: newNumber++, text, type: 'context' }
      })
    }
  }
  flush()

  if (hunks.length === 0) return { status: 'empty' }
  return { status: 'ok', hunks, added, removed }
}

export interface RefNames {
  local: Set<string>
  remote: Set<string>
}

function parseRefs(decorations: string, names: RefNames): GitRef[] {
  const refs: GitRef[] = []
  for (const entry of decorations.split(', ').filter(Boolean)) {
    if (entry.startsWith('tag: ')) refs.push({ name: entry.slice(5), kind: 'tag' })
    else if (entry.startsWith('HEAD -> ')) {
      refs.push({ name: 'HEAD', kind: 'head' }, { name: entry.slice(8), kind: 'branch' })
    } else if (entry === 'HEAD') refs.push({ name: 'HEAD', kind: 'head' })
    else if (names.local.has(entry)) refs.push({ name: entry, kind: 'branch' })
    else if (names.remote.has(entry)) refs.push({ name: entry, kind: 'remote' })
  }
  return refs
}

// Parses the log format `%H %P %an %ct %D %s` with unit separators between fields and a record
// separator after each commit.
export function parseLog(raw: string, names: RefNames, unpushed: Set<string>): GitCommit[] {
  const commits: GitCommit[] = []
  for (const record of raw.split('\x1e')) {
    const text = record.replace(/^\n/, '')
    if (!text) continue
    const [hash, parents, author, time, decorations, subject] = text.split('\x1f')
    commits.push({
      hash,
      parents: parents ? parents.split(' ') : [],
      author,
      time: Number(time),
      subject,
      refs: parseRefs(decorations, names),
      unpushed: unpushed.has(hash)
    })
  }
  return commits
}

const BLAME_HEADER = /^([0-9a-f]{40,64}) \d+ (\d+)/

// Parses `git blame --porcelain`. Details of a commit come only with its first line.
export function parseBlame(raw: string): GitBlameLine[] {
  const people = new Map<string, { author: string; time: number }>()
  const lines: GitBlameLine[] = []
  let hash = ''
  let number = 0
  for (const line of raw.split('\n')) {
    const header = BLAME_HEADER.exec(line)
    if (header) {
      hash = header[1]
      number = Number(header[2])
      if (!people.has(hash)) people.set(hash, { author: '', time: 0 })
    } else if (line.startsWith('author ')) {
      people.get(hash)!.author = line.slice(7)
    } else if (line.startsWith('author-time ')) {
      people.get(hash)!.time = Number(line.slice(12))
    } else if (line.startsWith('\t')) {
      const person = people.get(hash)!
      lines.push({ number, hash, author: person.author, time: person.time, text: line.slice(1) })
    }
  }
  return lines
}

// Parses `%(upstream:track)`, such as "[ahead 2, behind 1]". An upstream that is in sync or gone
// has no counts.
export function parseTrack(track: string): { ahead: number; behind: number } {
  const ahead = /ahead (\d+)/.exec(track)
  const behind = /behind (\d+)/.exec(track)
  return { ahead: ahead ? Number(ahead[1]) : 0, behind: behind ? Number(behind[1]) : 0 }
}

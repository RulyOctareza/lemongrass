// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { GitFileGroup, ProjectOpError } from '../../preload/types'

// What Cut or Copy put aside, for Paste. It lives in the renderer only and belongs to one project.
export const explorerClipboard = reactive<{
  projectPath: string | null
  mode: 'cut' | 'copy'
  paths: string[]
}>({ projectPath: null, mode: 'copy', paths: [] })

export function putOnClipboard(projectPath: string, mode: 'cut' | 'copy', paths: string[]): void {
  explorerClipboard.projectPath = projectPath
  explorerClipboard.mode = mode
  explorerClipboard.paths = paths
}

export function clearClipboard(): void {
  explorerClipboard.projectPath = null
  explorerClipboard.paths = []
}

const OP_ERRORS: Record<ProjectOpError, string> = {
  exists: 'A file or folder with that name is already there.',
  notFound: 'That file or folder no longer exists.',
  outside: 'That path is outside the project.',
  invalidName: 'That name cannot be used here.',
  intoItself: 'A folder cannot go into itself.',
  failed: 'The file operation failed.'
}

export function opErrorText(error: ProjectOpError): string {
  return OP_ERRORS[error]
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`
}

// The git panel's letters and colors for a change, so the explorer reads the same way.
export const GIT_MARKS: Record<GitFileGroup, { letter: string; color: string; title: string }> = {
  conflicted: { letter: '!', color: 'var(--color-warning)', title: 'Conflicted' },
  changed: { letter: 'M', color: 'var(--color-info)', title: 'Modified' },
  renamed: { letter: 'R', color: 'var(--color-info)', title: 'Renamed' },
  stagedNew: { letter: 'A', color: 'var(--color-success)', title: 'Added' },
  unstagedNew: { letter: 'U', color: 'var(--color-error)', title: 'Untracked' },
  deleted: { letter: 'D', color: 'var(--color-deleted)', title: 'Deleted' }
}

// A folder shows the most pressing change below it.
const GIT_RANK: GitFileGroup[] = [
  'conflicted',
  'unstagedNew',
  'changed',
  'renamed',
  'stagedNew',
  'deleted'
]

export function gitMarksByPath(
  files: { path: string; group: GitFileGroup }[]
): Map<string, GitFileGroup> {
  const marks = new Map<string, GitFileGroup>()
  const stronger = (a: GitFileGroup, b: GitFileGroup | undefined): boolean =>
    b === undefined || GIT_RANK.indexOf(a) < GIT_RANK.indexOf(b)
  for (const file of files) {
    marks.set(file.path, file.group)
    for (let at = file.path.lastIndexOf('/'); at > 0; at = file.path.lastIndexOf('/', at - 1)) {
      const dir = file.path.slice(0, at)
      if (stronger(file.group, marks.get(dir))) marks.set(dir, file.group)
    }
  }
  return marks
}

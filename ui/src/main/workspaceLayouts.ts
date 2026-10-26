// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain } from 'electron'
import { randomUUID } from 'crypto'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'fs'
import { homedir } from 'os'
import { join } from 'path'

const STORE_DIR = join(homedir(), '.lemongrass')
const STORE_FILE = join(STORE_DIR, 'workspace-layouts.json')
const LEGACY_SHELL_FILE = join(STORE_DIR, 'layouts.json')
const LEGACY_DOC_FILE = join(STORE_DIR, 'doc-layouts.json')

interface LegacyShellTab {
  id: string
  label: string
  command: string
  cwd?: string
}

interface LegacyDocTab {
  id: string
  path: string
}

type LegacyNode<T extends { id: string }> =
  | { type: 'leaf'; id: string; tabs: T[]; activeTabId: string | null }
  | {
      type: 'split'
      id: string
      direction: 'row' | 'column'
      children: LegacyNode<T>[]
      sizes: number[]
    }

interface WorkspaceState {
  root: LegacyNode<{ id: string; kind: string }> | null
  focusedPaneId: string | null
  groupColors?: Record<string, string>
}

function readJson(file: string): Record<string, unknown> {
  if (!existsSync(file)) return {}
  try {
    return JSON.parse(readFileSync(file, 'utf-8'))
  } catch {
    return {}
  }
}

function saveAll(layouts: Record<string, unknown>): void {
  mkdirSync(STORE_DIR, { recursive: true })
  writeFileSync(STORE_FILE, JSON.stringify(layouts, null, 2))
}

function mapTabs<T extends { id: string }, U extends { id: string }>(
  node: LegacyNode<T>,
  convert: (tab: T) => U
): LegacyNode<U> {
  if (node.type === 'leaf') return { ...node, tabs: node.tabs.map(convert) }
  return { ...node, children: node.children.map((child) => mapTabs(child, convert)) }
}

function firstLeafId(node: LegacyNode<{ id: string }>): string {
  return node.type === 'leaf' ? node.id : firstLeafId(node.children[0])
}

// Combines the terminal layout and the document layout saved before shells and
// documents shared one tree into a single tree, shells on the left.
function importLegacy(projectId: string): WorkspaceState | null {
  const shells = readJson(LEGACY_SHELL_FILE)[projectId] as LegacyNode<LegacyShellTab> | undefined
  const docs = readJson(LEGACY_DOC_FILE)[projectId] as
    { root: LegacyNode<LegacyDocTab> | null; focusedPaneId: string | null } | undefined

  const shellRoot = shells ? mapTabs(shells, (tab) => ({ ...tab, kind: 'shell' })) : null
  const docRoot = docs?.root ? mapTabs(docs.root, (tab) => ({ ...tab, kind: 'doc' })) : null

  if (shellRoot && docRoot) {
    return {
      root: {
        type: 'split',
        id: randomUUID(),
        direction: 'row',
        children: [shellRoot, docRoot],
        sizes: [0.5, 0.5]
      },
      focusedPaneId: docs?.focusedPaneId ?? firstLeafId(shellRoot)
    }
  }
  if (docRoot) return { root: docRoot, focusedPaneId: docs?.focusedPaneId ?? firstLeafId(docRoot) }
  if (shellRoot) return { root: shellRoot, focusedPaneId: firstLeafId(shellRoot) }
  return null
}

export function registerWorkspaceLayoutHandlers(): void {
  ipcMain.handle('workspaceLayouts:load', (_event, projectId: string) => {
    const saved = readJson(STORE_FILE)
    if (projectId in saved) return saved[projectId]
    return importLegacy(projectId)
  })

  ipcMain.on(
    'workspaceLayouts:save',
    (_event, { projectId, layout }: { projectId: string; layout: unknown }) => {
      const all = readJson(STORE_FILE)
      all[projectId] = layout
      saveAll(all)
    }
  )
}

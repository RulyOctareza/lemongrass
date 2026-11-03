// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { ProjectEntry } from '../../preload/types'
import type { ProjectRef } from './workspace'

export interface ExplorerState {
  // Loaded folder listings by project-relative folder path, '' for the root.
  listings: Record<string, ProjectEntry[]>
  // Expanded folders. A collapsed folder keeps its expanded children, so reopening it restores them.
  expanded: Record<string, true>
  selected: string | null
  rootMissing: boolean
}

const states = reactive<Record<string, ExplorerState>>({})
// The Biblio view's expanded folders by project, paths relative to biblio/. Kept here so they
// outlive the panel, which unmounts while the Files view shows.
const biblioExpanded = reactive<Record<string, Record<string, true>>>({})

export function biblioExpandedOf(projectId: string): Record<string, true> {
  if (!biblioExpanded[projectId]) biblioExpanded[projectId] = {}
  return biblioExpanded[projectId]
}
const projects = new Map<string, ProjectRef>()
// Projects whose watchers are live, by project path. Main watches each listed folder.
const attached = new Set<string>()

export function explorerOf(projectId: string): ExplorerState {
  if (!states[projectId]) {
    states[projectId] = { listings: {}, expanded: {}, selected: null, rootMissing: false }
  }
  return states[projectId]
}

export function parentOf(path: string): string {
  const slash = path.lastIndexOf('/')
  return slash === -1 ? '' : path.slice(0, slash)
}

export function baseName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

function isUnder(path: string, dir: string): boolean {
  return dir === '' || path === dir || path.startsWith(`${dir}/`)
}

// A folder shows only when every folder above it is expanded.
function isVisible(state: ExplorerState, dir: string): boolean {
  for (let at = parentOf(dir); at !== ''; at = parentOf(at)) {
    if (!state.expanded[at]) return false
  }
  return true
}

async function listDir(project: ProjectRef, dir: string): Promise<void> {
  const result = await window.api.projectFiles.list(project.path, dir)
  const state = explorerOf(project.id)
  if (!attached.has(project.path)) return
  if ('error' in result) {
    delete state.listings[dir]
    if (dir === '') state.rootMissing = true
    else delete state.expanded[dir]
    return
  }
  if (dir === '') state.rootMissing = false
  else if (!state.expanded[dir] || !isVisible(state, dir)) return
  state.listings[dir] = result.entries
}

// Lists the root and every visible expanded folder, which also starts their watches.
export async function attachExplorer(project: ProjectRef): Promise<void> {
  projects.set(project.path, project)
  if (attached.has(project.path)) return
  attached.add(project.path)
  const state = explorerOf(project.id)
  const dirs = ['', ...Object.keys(state.expanded).filter((dir) => isVisible(state, dir))]
  await Promise.all(dirs.map((dir) => listDir(project, dir)))
}

// Drops every watch for the project and the listings with them; the expanded set stays.
export function detachExplorer(project: ProjectRef): void {
  if (!attached.delete(project.path)) return
  window.api.projectFiles.unwatch(project.path, '')
  explorerOf(project.id).listings = {}
}

export function toggleDir(project: ProjectRef, dir: string): void {
  const state = explorerOf(project.id)
  if (state.expanded[dir]) collapseDir(project, dir)
  else void expandDir(project, dir)
}

export async function expandDir(project: ProjectRef, dir: string): Promise<void> {
  const state = explorerOf(project.id)
  state.expanded[dir] = true
  const reopened = Object.keys(state.expanded).filter(
    (path) => isUnder(path, dir) && isVisible(state, path)
  )
  await Promise.all(reopened.map((path) => listDir(project, path)))
}

export function collapseDir(project: ProjectRef, dir: string): void {
  const state = explorerOf(project.id)
  delete state.expanded[dir]
  for (const path of Object.keys(state.listings)) {
    if (path !== '' && isUnder(path, dir)) delete state.listings[path]
  }
  window.api.projectFiles.unwatch(project.path, dir)
}

export function collapseAll(project: ProjectRef): void {
  const state = explorerOf(project.id)
  for (const dir of Object.keys(state.expanded)) {
    if (!dir.includes('/')) window.api.projectFiles.unwatch(project.path, dir)
  }
  state.expanded = {}
  state.listings = { '': state.listings[''] ?? [] }
}

// Opens every folder above a path and selects it, for a search result or a new file.
export async function revealPath(project: ProjectRef, path: string): Promise<void> {
  const state = explorerOf(project.id)
  const dirs: string[] = []
  for (let at = parentOf(path); at !== ''; at = parentOf(at)) dirs.unshift(at)
  for (const dir of dirs) state.expanded[dir] = true
  state.selected = path
  await Promise.all(dirs.map((dir) => listDir(project, dir)))
}

export function selectPath(projectId: string, path: string | null): void {
  explorerOf(projectId).selected = path
}

let subscribed = false

// One listener for every project: a change relists that folder when it is loaded.
export function subscribeExplorerChanges(): void {
  if (subscribed) return
  subscribed = true
  window.api.projectFiles.onChanged((change) => {
    const project = projects.get(change.projectPath)
    if (!project || !attached.has(project.path)) return
    const state = explorerOf(project.id)
    if (change.dir === '' || state.listings[change.dir]) void listDir(project, change.dir)
  })
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain, net, protocol, shell, WebContents } from 'electron'
import { realpathSync, watch, type FSWatcher } from 'fs'
import { pathToFileURL } from 'url'
import { readEnv, run } from './gitRun'
import {
  copyEntries,
  createFile,
  createFolder,
  duplicateEntry,
  imagePath,
  listDir,
  moveEntries,
  readProjectFile,
  renameEntry,
  resolveEntries,
  resolveEntry,
  resolveReal,
  searchProject,
  toRelative,
  type ProjectGit
} from './projectFilesCore'
import type {
  ProjectFileResult,
  ProjectFilesChange,
  ProjectListResult,
  ProjectOpResult,
  ProjectOpsResult,
  ProjectSearchResult
} from '../preload/types'

export const PROJECT_FILE_SCHEME = 'project-file'

const WATCH_DEBOUNCE_MS = 300
// Each watch is one inotify watch on Linux, a shared per-user budget, so a tree with thousands of
// expanded folders stops watching new ones rather than starving other apps.
const MAX_WATCHES = 1000

interface DirWatch {
  watcher: FSWatcher
  timer?: ReturnType<typeof setTimeout>
}

// projectPath -> project-relative dir -> watch. Only folders the renderer has listed are watched,
// non-recursively, because a recursive watch on a project root also walks node_modules.
const watches = new Map<string, Map<string, DirWatch>>()

function watchCount(): number {
  let count = 0
  for (const dirs of watches.values()) count += dirs.size
  return count
}

function send(sender: WebContents | undefined, change: ProjectFilesChange): void {
  if (sender && !sender.isDestroyed()) sender.send('projectFiles:changed', change)
}

function stopWatch(projectPath: string, dir: string): void {
  const dirs = watches.get(projectPath)
  const entry = dirs?.get(dir)
  if (!dirs || !entry) return
  clearTimeout(entry.timer)
  entry.watcher.close()
  dirs.delete(dir)
  if (dirs.size === 0) watches.delete(projectPath)
}

function watchDir(
  projectPath: string,
  root: string,
  dir: string,
  getSender: () => WebContents | undefined
): void {
  if (watches.get(projectPath)?.has(dir) || watchCount() >= MAX_WATCHES) return
  const absolute = resolveReal(root, dir)
  if (!absolute) return
  let dirs = watches.get(projectPath)
  try {
    const entry: DirWatch = {
      watcher: watch(absolute, (_event, filename) => {
        // The root watch also sees .git and biblio change, which never show in this tree.
        if (dir === '' && (filename === '.git' || filename === 'biblio')) return
        clearTimeout(entry.timer)
        entry.timer = setTimeout(() => send(getSender(), { projectPath, dir }), WATCH_DEBOUNCE_MS)
      })
    }
    // A watched folder that is deleted or renamed errors here; its parent's watch reports it.
    entry.watcher.on('error', () => stopWatch(projectPath, dir))
    if (!dirs) {
      dirs = new Map()
      watches.set(projectPath, dirs)
    }
    dirs.set(dir, entry)
  } catch {
    // An unwatchable folder still lists; it just refreshes on demand.
  }
}

function unwatchTree(projectPath: string, dir: string): void {
  const dirs = watches.get(projectPath)
  if (!dirs) return
  for (const watched of [...dirs.keys()]) {
    if (dir === '' || watched === dir || watched.startsWith(dir + '/'))
      stopWatch(projectPath, watched)
  }
}

export function closeProjectFileWatchers(): void {
  for (const [projectPath, dirs] of watches) {
    for (const dir of [...dirs.keys()]) stopWatch(projectPath, dir)
  }
  watches.clear()
}

function realRoot(projectPath: string): string | null {
  try {
    return typeof projectPath === 'string' ? realpathSync(projectPath) : null
  } catch {
    return null
  }
}

function handleProjectFileRequest(request: Request): Response | Promise<Response> {
  try {
    const params = new URL(request.url).searchParams
    const projectPath = params.get('p')
    const relPath = params.get('f')
    if (!projectPath || relPath === null) return new Response(null, { status: 400 })
    const root = realRoot(projectPath)
    const target = root ? imagePath(root, relPath) : null
    if (!target) return new Response(null, { status: 403 })
    return net.fetch(pathToFileURL(target).toString())
  } catch {
    return new Response(null, { status: 404 })
  }
}

const noRoot = { error: 'notFound' } as const

// check-ignore refuses literal pathspec magic, which readEnv turns on; no call here passes a pathspec.
const projectGitEnv = { ...readEnv, GIT_LITERAL_PATHSPECS: '0' }

function gitIn(root: string): ProjectGit {
  return (args, input) => run(root, args, { env: projectGitEnv, input })
}

export function registerProjectFileHandlers(getSender: () => WebContents | undefined): void {
  // The scheme itself is registered as privileged in biblio.ts, since that call may run only once.
  protocol.handle(PROJECT_FILE_SCHEME, handleProjectFileRequest)

  ipcMain.handle(
    'projectFiles:list',
    async (_event, projectPath: string, dir: string): Promise<ProjectListResult> => {
      const root = realRoot(projectPath)
      if (!root) return noRoot
      const result = await listDir(root, dir, gitIn(root))
      if ('entries' in result) watchDir(projectPath, root, dir, getSender)
      return result
    }
  )

  ipcMain.on('projectFiles:unwatch', (_event, projectPath: string, dir: string) =>
    unwatchTree(projectPath, dir)
  )

  ipcMain.handle(
    'projectFiles:search',
    async (_event, projectPath: string, query: string): Promise<ProjectSearchResult> => {
      const root = realRoot(projectPath)
      if (!root || typeof query !== 'string') return { paths: [], truncated: false }
      return searchProject(root, query, gitIn(root))
    }
  )

  ipcMain.handle(
    'projectFiles:readFile',
    (_event, projectPath: string, path: string): ProjectFileResult | null => {
      const root = realRoot(projectPath)
      return root ? readProjectFile(root, path) : null
    }
  )

  ipcMain.handle(
    'projectFiles:createFile',
    (_event, projectPath: string, dir: string, name: string): ProjectOpResult => {
      const root = realRoot(projectPath)
      return root ? createFile(root, dir, name) : noRoot
    }
  )

  ipcMain.handle(
    'projectFiles:createFolder',
    (_event, projectPath: string, dir: string, name: string): ProjectOpResult => {
      const root = realRoot(projectPath)
      return root ? createFolder(root, dir, name) : noRoot
    }
  )

  ipcMain.handle(
    'projectFiles:rename',
    (_event, projectPath: string, path: string, newName: string): ProjectOpResult => {
      const root = realRoot(projectPath)
      return root ? renameEntry(root, path, newName) : noRoot
    }
  )

  ipcMain.handle(
    'projectFiles:move',
    (_event, projectPath: string, paths: string[], destDir: string): ProjectOpsResult => {
      const root = realRoot(projectPath)
      if (!root || !Array.isArray(paths)) return { ...noRoot, done: [] }
      return moveEntries(root, paths, destDir)
    }
  )

  ipcMain.handle(
    'projectFiles:copy',
    (_event, projectPath: string, paths: string[], destDir: string): ProjectOpsResult => {
      const root = realRoot(projectPath)
      if (!root || !Array.isArray(paths)) return { ...noRoot, done: [] }
      return copyEntries(root, paths, destDir)
    }
  )

  ipcMain.handle(
    'projectFiles:duplicate',
    (_event, projectPath: string, path: string): ProjectOpResult => {
      const root = realRoot(projectPath)
      return root ? duplicateEntry(root, path) : noRoot
    }
  )

  ipcMain.handle(
    'projectFiles:trash',
    async (_event, projectPath: string, paths: string[]): Promise<ProjectOpsResult> => {
      const root = realRoot(projectPath)
      if (!root || !Array.isArray(paths)) return { ...noRoot, done: [] }
      const resolved = resolveEntries(root, paths)
      if ('error' in resolved) return { error: resolved.error, done: [] }
      const done: string[] = []
      for (const target of resolved.targets) {
        try {
          await shell.trashItem(target)
        } catch {
          return { error: 'failed', done }
        }
        done.push(toRelative(root, target))
      }
      return { ok: true, paths: done }
    }
  )

  ipcMain.on('projectFiles:reveal', (_event, projectPath: string, path: string) => {
    const root = realRoot(projectPath)
    const target = root ? (path === '' ? root : resolveEntry(root, path)) : null
    if (target) shell.showItemInFolder(target)
  })
}

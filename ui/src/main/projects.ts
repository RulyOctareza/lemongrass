// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { BrowserWindow, dialog, ipcMain } from 'electron'
import { existsSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from 'fs'
import { homedir } from 'os'
import { basename, join, sep } from 'path'
import { randomUUID } from 'crypto'
import { writeTrust } from './projectTrust'
import { liveShellCwds } from './pty'
import type { ProjectDeleteResult } from '../preload/types'

export interface Project {
  id: string
  name: string
  path: string
  // Absent until the human answers the trust question for this project.
  trusted?: boolean
  // Set when the human deletes the project. The entry and lemongrass's data for it stay until projectPurge removes
  // them, so adding the folder again before then restores it under the same id.
  deletedAt?: string
}

// Centralized under ~/.lemongrass/, not Electron's userData folder, so the CLI and UI share one registry.
const STORE_DIR = join(homedir(), '.lemongrass')
const STORE_FILE = join(STORE_DIR, 'projects.json')

export function loadProjects(): Project[] {
  if (!existsSync(STORE_FILE)) return []
  try {
    return JSON.parse(readFileSync(STORE_FILE, 'utf-8'))
  } catch {
    return []
  }
}

export function saveProjects(projects: Project[]): void {
  mkdirSync(STORE_DIR, { recursive: true })
  writeFileSync(STORE_FILE, JSON.stringify(projects, null, 2))
}

// Mirrors project.resolvePath on the lgrass CLI side, so a symlinked path registered from either converges on the same entry.
function resolvePath(path: string): string {
  try {
    return realpathSync(path)
  } catch {
    return path
  }
}

function listedProjects(): Project[] {
  return loadProjects().filter((p) => !p.deletedAt)
}

export function isTrustedProject(path: string): boolean {
  const target = resolvePath(path)
  return listedProjects().some((p) => p.trusted === true && resolvePath(p.path) === target)
}

// Counts the shells running at or under each project's folder, whatever their state, so a project is never deleted
// with an agent still running for it.
function liveShellsByProject(projects: Project[]): Record<string, number> {
  const cwds = liveShellCwds().map(resolvePath)
  const counts: Record<string, number> = {}
  for (const p of projects) {
    const root = resolvePath(p.path)
    const n = cwds.filter((cwd) => cwd === root || cwd.startsWith(root + sep)).length
    if (n > 0) counts[p.id] = n
  }
  return counts
}

export function registerProjectHandlers(getWindow: () => BrowserWindow | undefined): void {
  ipcMain.handle('projects:list', () => listedProjects())

  ipcMain.handle('projects:home', () => homedir())

  ipcMain.handle('projects:liveShells', () => liveShellsByProject(listedProjects()))

  ipcMain.handle('projects:delete', (_event, id: string): ProjectDeleteResult | null => {
    const projects = loadProjects()
    const project = projects.find((p) => p.id === id && !p.deletedAt)
    if (!project) return null
    const liveShells = liveShellsByProject([project])[id] ?? 0
    if (liveShells > 0) return { ok: false, liveShells }
    project.deletedAt = new Date().toISOString()
    saveProjects(projects)
    return { ok: true }
  })

  ipcMain.handle('projects:add', async () => {
    const win = getWindow()
    const result = win
      ? await dialog.showOpenDialog(win, { properties: ['openDirectory'] })
      : await dialog.showOpenDialog({ properties: ['openDirectory'] })
    if (result.canceled || result.filePaths.length === 0) return null

    const dirPath = resolvePath(result.filePaths[0])
    const projects = loadProjects()
    const existing = projects.find((p) => resolvePath(p.path) === dirPath)
    if (existing?.deletedAt) {
      delete existing.deletedAt
      saveProjects(projects)
    }
    if (existing) return existing

    const project: Project = { id: randomUUID(), name: basename(dirPath), path: dirPath }
    projects.push(project)
    saveProjects(projects)
    return project
  })

  ipcMain.handle('projects:setTrust', async (_event, id: string, trusted: boolean) => {
    const projects = loadProjects()
    const project = projects.find((p) => p.id === id && !p.deletedAt)
    if (!project || typeof trusted !== 'boolean') return null
    project.trusted = trusted
    saveProjects(projects)
    if (trusted) await writeTrust(project.path)
    return project
  })
}

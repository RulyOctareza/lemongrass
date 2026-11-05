// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { execFile } from 'child_process'
import type { TabBusy } from '../preload/types'

const LGRASS_TIMEOUT_MS = 5_000

function runLgrassd(lgrassdPath: string, projectPath: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      lgrassdPath,
      ['tabs', ...args],
      { cwd: projectPath, encoding: 'utf8', timeout: LGRASS_TIMEOUT_MS, windowsHide: true },
      (failure, stdout) => (failure ? reject(failure) : resolve(stdout))
    )
  })
}

let registeredLgrassdPath: string | null = null

export async function registerTab(
  projectPath: string,
  tabId: string,
  vendor: string
): Promise<void> {
  if (!registeredLgrassdPath) return
  try {
    await runLgrassd(registeredLgrassdPath, projectPath, ['register', tabId, vendor])
  } catch (err) {
    console.error('lgrassd tabs register failed:', err)
  }
}

const BUSY_REASONS: Record<string, string> = {
  working: 'Mid-turn',
  prompting: 'Permission prompt open'
}
let statesFailureLogged = false

// Tabs with no recorded state are left out and count as not busy. An lgrassd built before
// `tabs states` existed fails here, which also reads as nobody busy.
export async function tabStates(projectPath: string): Promise<Record<string, TabBusy>> {
  if (!registeredLgrassdPath) return {}
  let states: Record<string, string>
  try {
    states = JSON.parse(await runLgrassd(registeredLgrassdPath, projectPath, ['states']))
  } catch (err) {
    // The picker polls this, so a lasting failure is logged once.
    if (!statesFailureLogged) console.error('lgrassd tabs states failed:', err)
    statesFailureLogged = true
    return {}
  }
  const out: Record<string, TabBusy> = {}
  for (const [tabId, state] of Object.entries(states ?? {})) {
    const reason = BUSY_REASONS[state] ?? null
    out[tabId] = { busy: reason !== null, reason }
  }
  return out
}

export function registerTabSessionHandlers(lgrassdPath: string | null): void {
  registeredLgrassdPath = lgrassdPath
  ipcMain.handle(
    'tabSessions:list',
    async (_event, projectPath: string): Promise<Record<string, string>> => {
      if (!lgrassdPath) return {}
      try {
        return JSON.parse(await runLgrassd(lgrassdPath, projectPath, ['list']))
      } catch (err) {
        console.error('lgrassd tabs list failed:', err)
        return {}
      }
    }
  )

  ipcMain.handle('tabSessions:states', (_event, projectPath: string) => tabStates(projectPath))

  ipcMain.handle('tabSessions:clear', async (_event, projectPath: string) => {
    if (!lgrassdPath) return
    try {
      await runLgrassd(lgrassdPath, projectPath, ['clear'])
    } catch (err) {
      console.error('lgrassd tabs clear failed:', err)
    }
  })

  ipcMain.handle(
    'tabSessions:title',
    async (
      _event,
      { projectPath, tabId, title }: { projectPath: string; tabId: string; title: string }
    ) => {
      if (!lgrassdPath) return
      try {
        await runLgrassd(lgrassdPath, projectPath, ['title', tabId, title])
      } catch (err) {
        console.error('lgrassd tabs title failed:', err)
      }
    }
  )

  ipcMain.handle(
    'tabSessions:forget',
    async (_event, { projectPath, tabId }: { projectPath: string; tabId: string }) => {
      if (!lgrassdPath) return
      try {
        await runLgrassd(lgrassdPath, projectPath, ['forget', tabId])
      } catch (err) {
        console.error('lgrassd tabs forget failed:', err)
      }
    }
  )
}

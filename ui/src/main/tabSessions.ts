// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { execFile } from 'child_process'

const LGRASS_TIMEOUT_MS = 5_000

function runLgrassd(lgrassdPath: string, projectPath: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      lgrassdPath,
      ['tabs', ...args],
      { cwd: projectPath, encoding: 'utf8', timeout: LGRASS_TIMEOUT_MS },
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

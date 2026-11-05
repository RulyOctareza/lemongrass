// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain, type WebContents } from 'electron'
import { execFile } from 'child_process'
import { watch, type FSWatcher } from 'fs'
import type { ContextUsage, StatusLineChainState, UsageReport } from '../preload/types'

const LGRASSD_TIMEOUT_MS = 10_000

function runJson<T>(lgrassdPath: string, args: string[]): Promise<T | null> {
  return new Promise((resolve) => {
    execFile(
      lgrassdPath,
      args,
      { encoding: 'utf8', timeout: LGRASSD_TIMEOUT_MS, windowsHide: true },
      (failure, stdout) => {
        if (failure) return resolve(null)
        try {
          resolve(JSON.parse(stdout) as T)
        } catch {
          resolve(null)
        }
      }
    )
  })
}

const CONTEXT_DEBOUNCE_MS = 150

interface ContextReport {
  dir: string
  tabs: Record<string, ContextUsage>
}

// Tab context figures change on every Claude response, too often for the usage poll, so the directory lgrassd keeps
// them in is watched and each change is pushed to the renderer. lgrassd names the directory, so its layout stays in
// Go.
function contextFeed(
  lgrassdPath: string | null,
  getSender: () => WebContents | undefined
): () => Promise<Record<string, ContextUsage> | null> {
  let watcher: FSWatcher | undefined
  let timer: ReturnType<typeof setTimeout> | undefined

  const read = async (): Promise<Record<string, ContextUsage> | null> => {
    if (!lgrassdPath) return null
    const report = await runJson<ContextReport>(lgrassdPath, ['usage', 'context'])
    if (!report) return null
    if (!watcher) {
      try {
        watcher = watch(report.dir, () => {
          clearTimeout(timer)
          timer = setTimeout(push, CONTEXT_DEBOUNCE_MS)
        })
        // A watch that fails is dropped, and the next read starts a fresh one.
        watcher.on('error', () => {
          watcher?.close()
          watcher = undefined
        })
      } catch {
        watcher = undefined
      }
    }
    return report.tabs
  }

  const push = async (): Promise<void> => {
    const tabs = await read()
    const sender = getSender()
    if (tabs && sender && !sender.isDestroyed()) sender.send('usage:context-changed', tabs)
  }

  return read
}

// The figures come from `lgrassd usage`, so the layout of the usage files and of Codex's rollouts stays in Go.
export function registerUsageHandlers(
  lgrassdPath: string | null,
  getSender: () => WebContents | undefined
): void {
  const readContexts = contextFeed(lgrassdPath, getSender)
  ipcMain.handle('usage:context', () => readContexts())

  ipcMain.handle('usage:get', async (): Promise<UsageReport | null> =>
    lgrassdPath ? runJson<UsageReport>(lgrassdPath, ['usage']) : null
  )

  ipcMain.handle('statusline:state', async (): Promise<StatusLineChainState | null> =>
    lgrassdPath ? runJson<StatusLineChainState>(lgrassdPath, ['statusline-chain']) : null
  )

  ipcMain.handle(
    'statusline:set',
    async (_event, enabled: boolean): Promise<StatusLineChainState | null> =>
      lgrassdPath
        ? runJson<StatusLineChainState>(lgrassdPath, ['statusline-chain', enabled ? 'on' : 'off'])
        : null
  )
}

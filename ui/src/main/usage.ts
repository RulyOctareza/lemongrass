// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { execFile } from 'child_process'
import type { StatusLineChainState, UsageReport } from '../preload/types'

const LGRASSD_TIMEOUT_MS = 10_000

function runJson<T>(lgrassdPath: string, args: string[]): Promise<T | null> {
  return new Promise((resolve) => {
    execFile(
      lgrassdPath,
      args,
      { encoding: 'utf8', timeout: LGRASSD_TIMEOUT_MS },
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

// The figures come from `lgrassd usage`, so the layout of the usage files and of Codex's rollouts stays in Go.
export function registerUsageHandlers(lgrassdPath: string | null): void {
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

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { execFile } from 'child_process'
import type { UsageReport } from '../preload/types'

const LGRASSD_TIMEOUT_MS = 10_000

// The figures come from `lgrassd usage`, so the layout of the usage files and of Codex's rollouts stays in Go.
export function registerUsageHandlers(lgrassdPath: string | null): void {
  ipcMain.handle('usage:get', (): Promise<UsageReport | null> => {
    if (!lgrassdPath) return Promise.resolve(null)
    return new Promise((resolve) => {
      execFile(
        lgrassdPath,
        ['usage'],
        { encoding: 'utf8', timeout: LGRASSD_TIMEOUT_MS },
        (failure, stdout) => {
          if (failure) return resolve(null)
          try {
            resolve(JSON.parse(stdout) as UsageReport)
          } catch {
            resolve(null)
          }
        }
      )
    })
  })
}

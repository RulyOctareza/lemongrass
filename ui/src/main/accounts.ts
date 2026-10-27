// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain } from 'electron'
import { execFile } from 'child_process'
import type { AccountAddResult, AccountInfo, AccountRemoveResult } from '../preload/types'

const LGRASSD_TIMEOUT_MS = 10_000
const NOT_INSTALLED = 'accounts: the lgrassd binary is not installed'

function runLgrassd(lgrassdPath: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      lgrassdPath,
      ['accounts', ...args],
      { encoding: 'utf8', timeout: LGRASSD_TIMEOUT_MS },
      (failure, stdout, stderr) => {
        if (!failure) return resolve(stdout)
        reject(new Error(stderr.trim().replace(/^error: /, '') || failure.message))
      }
    )
  })
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

export function registerAccountHandlers(lgrassdPath: string | null): void {
  ipcMain.handle('accounts:list', async (): Promise<AccountInfo[]> => {
    if (!lgrassdPath) throw new Error(NOT_INSTALLED)
    return JSON.parse(await runLgrassd(lgrassdPath, ['list']))
  })

  ipcMain.handle('accounts:add', async (_event, name: string): Promise<AccountAddResult> => {
    if (!lgrassdPath) return { ok: false, error: NOT_INSTALLED }
    try {
      return { ok: true, account: JSON.parse(await runLgrassd(lgrassdPath, ['add', name])) }
    } catch (err) {
      return { ok: false, error: messageOf(err) }
    }
  })

  ipcMain.handle('accounts:remove', async (_event, name: string): Promise<AccountRemoveResult> => {
    if (!lgrassdPath) return { ok: false, error: NOT_INSTALLED }
    try {
      await runLgrassd(lgrassdPath, ['remove', name])
      return { ok: true }
    } catch (err) {
      return { ok: false, error: messageOf(err) }
    }
  })
}

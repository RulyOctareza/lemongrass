// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app, BrowserWindow, ipcMain } from 'electron'
import { execFile } from 'child_process'
import { existsSync } from 'fs'
import type { UninstallState } from '../preload/types'
import { resolveBundledBin } from './lgrassInstall'
import { runUntilExec } from './updateRun'
import { unsupportedReason } from './updaterRpm'

const RPM = '/usr/bin/rpm'

let currentWindow: () => BrowserWindow | undefined = () => undefined
let state: UninstallState = { phase: 'unavailable', message: null, output: null }

function set(patch: Partial<UninstallState>): void {
  state = { ...state, ...patch }
  currentWindow()?.webContents.send('uninstall:state-changed', state)
}

// The bundled copy is used rather than the one on PATH, which uninstall itself removes.
function bundledLgrassconf(): string | null {
  const path = resolveBundledBin('lgrassconf')
  return path && existsSync(path) ? path : null
}

function clean(binary: string): Promise<{ ok: boolean; output: string }> {
  return new Promise((resolve) => {
    execFile(binary, ['uninstall'], { encoding: 'utf8' }, (failure, stdout, stderr) => {
      resolve({ ok: !failure, output: `${stdout}${stderr}`.trim() })
    })
  })
}

// The per-user cleanup runs before the package goes, so a dismissed password prompt leaves only a cleaned account,
// which the next launch sets up again, and the app never keeps running on a deleted /opt.
async function uninstall(): Promise<void> {
  if (!['idle', 'failed'].includes(state.phase)) return
  const binary = bundledLgrassconf()
  if (!binary) {
    set({ phase: 'failed', message: 'uninstall: the bundled lgrassconf is missing.', output: null })
    return
  }
  set({ phase: 'cleaning', message: null, output: null })
  const result = await clean(binary)
  if (!result.ok) {
    set({
      phase: 'failed',
      message: 'Cleaning up this account failed, so the package was not removed.',
      output: result.output
    })
    return
  }
  set({ phase: 'authorizing' })
  try {
    await runUntilExec('pkexec', [RPM, '-e', 'lemongrass'], () => set({ phase: 'removing' }))
  } catch (err) {
    const dismissed = (err as { code?: unknown }).code === 126
    set({
      phase: 'failed',
      message: dismissed
        ? 'The password prompt was dismissed, so the package stays. The next launch sets Lemongrass up again.'
        : `uninstall: ${err instanceof Error ? err.message : String(err)}`,
      output: result.output
    })
    return
  }
  app.quit()
}

export function registerUninstallHandlers(getWindow: () => BrowserWindow | undefined): void {
  currentWindow = getWindow
  ipcMain.handle('uninstall:state', (): UninstallState => state)
  ipcMain.handle('uninstall:run', (): void => {
    void uninstall()
  })
  if (process.platform !== 'linux' || !app.isPackaged || !bundledLgrassconf()) return
  void unsupportedReason().then((reason) => {
    if (!reason) set({ phase: 'idle' })
  })
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { BrowserWindow, ipcMain } from 'electron'

// frame: false means we draw our own header bar -- this is the IPC side
// of it: minimize/maximize/close, plus pushing maximize state so the
// renderer can swap the maximize/restore icon.
export function registerWindowControlHandlers(getWindow: () => BrowserWindow | undefined): void {
  ipcMain.on('window:minimize', () => getWindow()?.minimize())

  ipcMain.on('window:toggle-maximize', () => {
    const win = getWindow()
    if (!win) return
    if (win.isMaximized()) win.unmaximize()
    else win.maximize()
  })

  ipcMain.on('window:close', () => getWindow()?.close())

  ipcMain.handle('window:is-maximized', () => getWindow()?.isMaximized() ?? false)
}

export function wireMaximizeEvents(win: BrowserWindow): void {
  win.on('maximize', () => win.webContents.send('window:maximized', true))
  win.on('unmaximize', () => win.webContents.send('window:maximized', false))
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { BrowserWindow, BrowserWindowConstructorOptions, ipcMain } from 'electron'

// The header bar's CSS height in HeaderBar.vue. At zoom 1 a CSS pixel is one device pixel.
const HEADER_HEIGHT = 46
// Left margin of the first traffic light, and the height AppKit gives each light's button frame.
// Electron places the buttons' top edge at trafficLightPosition.y, so y is half the header minus
// half a button to center them.
const LIGHTS_X = 14
const LIGHT_BUTTON_HEIGHT = 16

function lightsPosition(headerHeight: number): { x: number; y: number } {
  return { x: LIGHTS_X, y: Math.round((headerHeight - LIGHT_BUTTON_HEIGHT) / 2) }
}

// Linux and Windows are frameless and the header draws its own controls. macOS keeps a frame,
// since a frameless window there loses the traffic lights; titleBarStyle 'hidden' removes the
// title bar but keeps the lights over the content, where the header leaves room for them.
export function windowChromeOptions(zoomFactor: number): BrowserWindowConstructorOptions {
  if (process.platform !== 'darwin') return { frame: false }
  return {
    titleBarStyle: 'hidden',
    trafficLightPosition: lightsPosition(HEADER_HEIGHT * zoomFactor)
  }
}

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
  ipcMain.handle('window:is-full-screen', () => getWindow()?.isFullScreen() ?? false)

  // The header's device height changes with the zoom factor, so the lights move with it.
  ipcMain.on('window:header-height', (_event, dip: number) => {
    const win = getWindow()
    if (process.platform !== 'darwin' || !win || !(dip > 0)) return
    win.setWindowButtonPosition(lightsPosition(dip))
  })
}

export function wireMaximizeEvents(win: BrowserWindow): void {
  win.on('maximize', () => win.webContents.send('window:maximized', true))
  win.on('unmaximize', () => win.webContents.send('window:maximized', false))
  // macOS hides the traffic lights in full screen, so the header drops the room it keeps for them.
  win.on('enter-full-screen', () => win.webContents.send('window:full-screen', true))
  win.on('leave-full-screen', () => win.webContents.send('window:full-screen', false))
}

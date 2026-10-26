// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { app, BrowserWindow, Notification } from 'electron'
import { basename } from 'path'

// Electron drops a notification's click listener once the object is garbage collected, which on Linux can happen before the click.
const live = new Set<Notification>()

function reveal(window: BrowserWindow): void {
  if (window.isDestroyed()) return
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
  app.focus({ steal: true })
  window.webContents.send('workgroup:open-approvals')
}

export function notifyApprovalWaiting(window: BrowserWindow, projectPath: string): void {
  if (!Notification.isSupported()) {
    window.flashFrame(true)
    return
  }
  const notification = new Notification({
    title: 'Workgroup approval',
    body: `An agent is waiting for a workgroup approval in ${basename(projectPath)}.`
  })
  live.add(notification)
  const release = (): void => void live.delete(notification)
  notification.on('click', () => {
    release()
    reveal(window)
  })
  notification.on('close', release)
  notification.on('failed', release)
  notification.show()
}

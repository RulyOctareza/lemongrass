// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app, BrowserWindow, Notification } from 'electron'
import { basename } from 'path'

// Electron drops a notification's click listener once the object is garbage collected, which on Linux can happen before the click.
const live = new Set<Notification>()

export interface AppNotice {
  title: string
  // Never model-supplied text: a notification is outside the app, so anything could spoof one.
  body: string
  // The renderer channel sent on click, after the window is brought to the front.
  channel: string
  payload?: unknown
}

function reveal(window: BrowserWindow, notice: AppNotice): void {
  if (window.isDestroyed()) return
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
  app.focus({ steal: true })
  window.webContents.send(notice.channel, notice.payload)
}

// Where notifications are not supported, a taskbar flash stands in and the sidebar badges carry the rest.
export function notify(window: BrowserWindow, notice: AppNotice): void {
  if (window.isDestroyed()) return
  if (!Notification.isSupported()) {
    window.flashFrame(true)
    return
  }
  const notification = new Notification({ title: notice.title, body: notice.body })
  live.add(notification)
  const release = (): void => void live.delete(notification)
  notification.on('click', () => {
    release()
    reveal(window, notice)
  })
  notification.on('close', release)
  notification.on('failed', release)
  notification.show()
}

export function notifyApprovalWaiting(window: BrowserWindow, projectPath: string): void {
  notify(window, {
    title: 'Workgroup approval',
    body: `An agent is waiting for a workgroup approval in ${basename(projectPath)}.`,
    channel: 'workgroup:open-approvals'
  })
}

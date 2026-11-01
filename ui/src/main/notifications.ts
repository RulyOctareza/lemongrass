// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { BrowserWindow, ipcMain } from 'electron'
import { randomUUID } from 'crypto'
import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from 'fs'
import { homedir } from 'os'
import { join } from 'path'
import type { Notice, NoticeEvent, NoticeInput, NoticeKind } from '../preload/types'
import type { AppNotice } from './appNotify'

// The notification center's items. Waiting decisions are not stored here: the renderer shows them
// live from the ask count and the pending approvals, and an answered one becomes a history item.

const STORE_DIR = join(homedir(), '.lemongrass')
const STORE_FILE = join(STORE_DIR, 'notifications.json')
const MAX_ITEMS = 200
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000
const MAX_TEXT = 300

const KINDS: Record<NoticeEvent, NoticeKind> = {
  'approval-answered': 'history',
  'ask-answered': 'history',
  'turn-ended': 'agent',
  'tab-prompting': 'agent',
  'conflict-submitted': 'agent',
  'update-available': 'app',
  'dep-problem': 'app',
  'vault-locked': 'app',
  'git-failed': 'app',
  popup: 'app'
}

// Popups for these channels stand for a waiting decision, which the center already lists live.
const DECISION_CHANNELS = new Set(['ask:open', 'workgroup:open-approvals'])

let items: Notice[] = []
let currentWindow: () => BrowserWindow | undefined = () => undefined

function prune(list: Notice[]): Notice[] {
  const cutoff = Date.now() - MAX_AGE_MS
  return list.filter((item) => item.createdAt >= cutoff).slice(0, MAX_ITEMS)
}

function load(): Notice[] {
  if (!existsSync(STORE_FILE)) return []
  try {
    const parsed = JSON.parse(readFileSync(STORE_FILE, 'utf-8'))
    if (!Array.isArray(parsed)) return []
    return prune(
      parsed.filter(
        (item): item is Notice =>
          item &&
          typeof item.id === 'string' &&
          item.event in KINDS &&
          typeof item.createdAt === 'number'
      )
    )
  } catch {
    return []
  }
}

// Written to a temporary file and renamed, so a crash mid-write never leaves half a file.
function save(): void {
  try {
    if (!existsSync(STORE_DIR)) mkdirSync(STORE_DIR, { recursive: true })
    const temp = `${STORE_FILE}.tmp`
    writeFileSync(temp, JSON.stringify(items))
    renameSync(temp, STORE_FILE)
  } catch (err) {
    console.error('notifications: saving the notification center failed:', err)
  }
}

function publish(): void {
  const window = currentWindow()
  if (window && !window.isDestroyed()) window.webContents.send('notifications:changed', items)
}

function text(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed ? trimmed.slice(0, MAX_TEXT) : undefined
}

function clean(input: NoticeInput): NoticeInput | null {
  if (!input || !(input.event in KINDS)) return null
  return {
    event: input.event,
    key: text(input.key),
    projectPath: text(input.projectPath),
    tabId: text(input.tabId),
    tabTitle: text(input.tabTitle),
    subject: text(input.subject),
    detail: text(input.detail),
    outcome: text(input.outcome),
    read: input.read === true
  }
}

export function recordNotification(input: NoticeInput): void {
  const fields = clean(input)
  if (!fields) return
  const existing = fields.key ? items.find((item) => item.key === fields.key) : undefined
  if (existing && input.once === true) return
  const item: Notice = {
    ...fields,
    id: existing?.id ?? randomUUID(),
    kind: KINDS[fields.event],
    createdAt: Date.now(),
    read: fields.read === true
  }
  items = prune([item, ...items.filter((other) => other !== existing)])
  save()
  publish()
}

// Every OS popup lands here, so dismissing the popup loses nothing.
export function recordNotice(notice: AppNotice): void {
  if (DECISION_CHANNELS.has(notice.channel)) return
  recordNotification({
    event: 'popup',
    key: `popup:${notice.title}:${notice.body}`,
    subject: notice.title,
    detail: notice.body
  })
}

function matches(ids: unknown): (item: Notice) => boolean {
  if (!Array.isArray(ids)) return () => true
  const wanted = new Set(ids.map(String))
  return (item) => wanted.has(item.id)
}

export function registerNotificationHandlers(getWindow: () => BrowserWindow | undefined): void {
  currentWindow = getWindow
  items = load()
  ipcMain.handle('notifications:list', (): Notice[] => items)
  // A popup is raised by main only, so the renderer cannot add one.
  ipcMain.handle('notifications:add', (_event, input: NoticeInput): void => {
    if (input?.event === 'popup') return
    recordNotification(input)
  })
  // No ids means every item.
  ipcMain.handle('notifications:markRead', (_event, ids?: string[]): void => {
    const pick = matches(ids)
    items = items.map((item) => (pick(item) && !item.read ? { ...item, read: true } : item))
    save()
    publish()
  })
  ipcMain.handle('notifications:clear', (_event, ids?: string[]): void => {
    const pick = matches(ids)
    items = items.filter((item) => !pick(item))
    save()
    publish()
  })
}

// Passes a git action's result through, recording a failure for the center.
export function noteGitFailure<T extends { ok: boolean; error?: string }>(
  projectPath: string,
  operation: 'fetch' | 'push',
  result: T
): T {
  if (!result.ok) {
    recordNotification({
      event: 'git-failed',
      key: `git-failed:${operation}:${projectPath}`,
      projectPath,
      detail: operation,
      subject: (result.error ?? '').split('\n').find((line) => line.trim()) ?? ''
    })
  }
  return result
}

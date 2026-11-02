// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { BrowserWindow, ipcMain } from 'electron'
import type { Ask, AskCount, AskDeclineExtra } from '../preload/types'
import { notify } from './appNotify'
import { vaultCall } from './vault'

// Asks live in the vault daemon. Main holds no passphrase, so it only reads the content-free count
// and forwards the renderer's root-secret calls; the daemon's hint on app.sock says when to look again.

const POLL_MS = 30_000
const REPEAT_MS = 60_000

let count: AskCount = { waiting: 0, blocking: 0, newestId: '' }
let notifiedId = ''
let currentWindow: () => BrowserWindow | undefined = () => undefined
let pollTimer: ReturnType<typeof setInterval> | undefined
let repeatTimer: ReturnType<typeof setInterval> | undefined

function send(channel: string, payload?: unknown): void {
  const window = currentWindow()
  if (window && !window.isDestroyed()) window.webContents.send(channel, payload)
}

// The body is fixed text: the notification sits outside the app, so it never carries model output.
function announce(): void {
  const window = currentWindow()
  if (!window || count.waiting === 0) return
  notify(window, {
    title: 'Decision waiting',
    body:
      count.waiting === 1
        ? 'An agent is waiting for your decision.'
        : `${count.waiting} agent requests are waiting for your decision.`,
    channel: 'ask:open',
    payload: count.newestId
  })
}

// Repeats only while a model's `lgrass ask` is still waiting on an answer.
function syncRepeat(): void {
  if (count.blocking > 0 && !repeatTimer) repeatTimer = setInterval(announce, REPEAT_MS)
  if (count.blocking === 0 && repeatTimer) {
    clearInterval(repeatTimer)
    repeatTimer = undefined
  }
}

async function readCount(): Promise<AskCount> {
  const reply = await vaultCall<{ waiting?: number; blocking?: number; newest_id?: string }>(
    'ask_count'
  )
  return {
    waiting: reply?.waiting ?? 0,
    blocking: reply?.blocking ?? 0,
    newestId: reply?.newest_id ?? ''
  }
}

async function refresh(hinted: boolean): Promise<void> {
  let next: AskCount
  try {
    next = await readCount()
  } catch {
    return
  }
  const changed =
    next.waiting !== count.waiting ||
    next.blocking !== count.blocking ||
    next.newestId !== count.newestId
  count = next
  if (changed) send('ask:count', count)
  if (hinted || changed) send('ask:changed')
  if (count.waiting > 0 && count.newestId && count.newestId !== notifiedId) {
    notifiedId = count.newestId
    announce()
  }
  syncRepeat()
}

// The daemon's hint carries no content, so a forged one only causes a refetch.
export async function handleAskHint(): Promise<{ ok: boolean }> {
  await refresh(true)
  return { ok: true }
}

export function startAsks(getWindow: () => BrowserWindow | undefined): void {
  currentWindow = getWindow
  ipcMain.handle('ask:count', () => count)
  ipcMain.handle('ask:list', async (_event, passphrase: string) => {
    const { asks } = await vaultCall<{ asks: Ask[] | null }>('list_asks', {
      root_secret: passphrase
    })
    return asks ?? []
  })
  ipcMain.handle('ask:get', async (_event, passphrase: string, id: string) => {
    const { ask } = await vaultCall<{ ask: Ask }>('get_ask', { root_secret: passphrase, id })
    return ask
  })
  ipcMain.handle(
    'ask:execute',
    async (_event, passphrase: string, id: string, timeoutSeconds: number) => {
      const { ask } = await vaultCall<{ ask: Ask }>('execute_ask', {
        root_secret: passphrase,
        id,
        timeout_seconds: timeoutSeconds
      })
      return ask
    }
  )
  ipcMain.handle('ask:stop', (_event, passphrase: string, id: string) =>
    vaultCall<void>('stop_ask', { root_secret: passphrase, id })
  )
  ipcMain.handle(
    'ask:decline',
    (_event, passphrase: string, id: string, note: string, extra?: AskDeclineExtra) =>
      vaultCall<void>('decline_ask', {
        root_secret: passphrase,
        id,
        note,
        ...(extra?.dbName ? { db_name: extra.dbName } : {}),
        ...(extra?.removedTables?.length ? { removed_tables: extra.removedTables } : {})
      })
  )
  void refresh(false)
  pollTimer = setInterval(() => void refresh(false), POLL_MS)
}

export function stopAsks(): void {
  if (pollTimer) clearInterval(pollTimer)
  if (repeatTimer) clearInterval(repeatTimer)
  pollTimer = undefined
  repeatTimer = undefined
}

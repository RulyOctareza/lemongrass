// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { BrowserWindow, ipcMain } from 'electron'
import { randomUUID } from 'crypto'
import type { WorkgroupPending, WorkgroupPendingMember } from '../preload/types'
import { cleanReason } from './workgroupReason'
import { cleanThinkerAccounts } from './accountNames'
import { notifyApprovalWaiting } from './workgroupNotify'

export interface ApprovalRequest {
  projectPath: string
  groupName: string
  leaderTabId: string
  leaderLabel: string
  // Present when the proposal adds thinkers to a live group: who is already in it.
  existingMembers?: string[]
  members: WorkgroupPendingMember[]
}

export interface ApprovalAnswer {
  approved: boolean
  reason?: string
  withdrawn?: boolean
  accounts?: string[]
}

interface Entry {
  info: WorkgroupPending
  settle: (answer: ApprovalAnswer) => void
}

const WITHDRAWN: ApprovalAnswer = { approved: false, withdrawn: true }

const entries = new Map<string, Entry>()
const watched = new WeakSet<BrowserWindow>()
let currentWindow: () => BrowserWindow | undefined = () => undefined

function list(): WorkgroupPending[] {
  return [...entries.values()].map((entry) => entry.info).sort((a, b) => a.createdAt - b.createdAt)
}

function publish(): void {
  const window = currentWindow()
  if (window && !window.isDestroyed()) window.webContents.send('workgroup:pending-changed', list())
}

function declineAll(): void {
  for (const entry of [...entries.values()]) entry.settle(WITHDRAWN)
}

function watch(window: BrowserWindow): void {
  if (watched.has(window)) return
  watched.add(window)
  window.once('closed', declineAll)
}

export function registerApprovalHandlers(getWindow: () => BrowserWindow | undefined): void {
  currentWindow = getWindow
  ipcMain.handle('workgroup:pending', (): WorkgroupPending[] => list())
  ipcMain.handle(
    'workgroup:decide',
    (
      _event,
      payload: { requestId: string; approved: boolean; reason?: string; accounts?: unknown }
    ): boolean => {
      const entry = entries.get(String(payload?.requestId))
      if (!entry) return false
      if (payload.approved === true) {
        const accounts = cleanThinkerAccounts(payload.accounts, entry.info.members.length)
        if (accounts === null) return false
        entry.settle({ approved: true, accounts })
      } else entry.settle({ approved: false, reason: cleanReason(payload.reason) })
      return true
    }
  )
}

// Resolves with the human's answer, or a withdrawal when the caller disconnects or the window closes first.
export function requestApproval(
  window: BrowserWindow,
  request: ApprovalRequest,
  signal: AbortSignal
): Promise<ApprovalAnswer> {
  return new Promise((resolve) => {
    const requestId = randomUUID()
    const onAbort = (): void => settle(WITHDRAWN)
    const settle = (answer: ApprovalAnswer): void => {
      if (!entries.delete(requestId)) return
      signal.removeEventListener('abort', onAbort)
      publish()
      resolve(answer)
    }
    if (signal.aborted) return resolve(WITHDRAWN)
    entries.set(requestId, { info: { requestId, ...request, createdAt: Date.now() }, settle })
    signal.addEventListener('abort', onAbort, { once: true })
    watch(window)
    publish()
    notifyApprovalWaiting(window, request.projectPath)
  })
}

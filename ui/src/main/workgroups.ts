// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { execFile } from 'child_process'
import type {
  WorkgroupInfo,
  WorkgroupJoinRequest,
  WorkgroupJoinResult,
  WorkgroupThread,
  WorkgroupThreadResult
} from '../preload/types'
import { typeSystemLine } from './tabNudge'
import { mintJoinToken } from './workgroupBridge'

const LGRASS_TIMEOUT_MS = 5_000
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const THREAD_ID = /^[a-z0-9]{6}$/

function runLgrass(lgrassPath: string, projectPath: string, args: string[]): Promise<string> {
  return runCommand(lgrassPath, projectPath, ['workgroup', ...args])
}

function runCommand(lgrassPath: string, projectPath: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      lgrassPath,
      args,
      { cwd: projectPath, encoding: 'utf8', timeout: LGRASS_TIMEOUT_MS },
      (failure, stdout, stderr) =>
        failure ? reject(new Error(stderr.trim() || failure.message)) : resolve(stdout)
    )
  })
}

export function registerWorkgroupHandlers(lgrassPath: string | null): void {
  ipcMain.handle(
    'workgroups:list',
    async (_event, projectPath: string): Promise<WorkgroupInfo[]> => {
      if (!lgrassPath) return []
      try {
        return JSON.parse(await runLgrass(lgrassPath, projectPath, ['list', '--json']))
      } catch (err) {
        console.error('lgrass workgroup list failed:', err)
        return []
      }
    }
  )

  ipcMain.handle(
    'workgroups:thread',
    async (
      _event,
      {
        projectPath,
        threadId,
        before,
        limit
      }: { projectPath: string; threadId: string; before?: number; limit?: number }
    ): Promise<WorkgroupThreadResult> => {
      if (!lgrassPath) return { ok: false, error: 'lgrass is not installed' }
      const args = ['thread', 'read', threadId, '--json']
      args.push('--limit', String(Math.min(Math.max(Math.trunc(limit ?? 50), 1), 200)))
      if (before && before > 0) args.push('--before', String(Math.trunc(before)))
      try {
        const thread: WorkgroupThread = JSON.parse(await runCommand(lgrassPath, projectPath, args))
        return { ok: true, thread }
      } catch (err) {
        return { ok: false, error: err instanceof Error ? err.message : String(err) }
      }
    }
  )

  ipcMain.handle(
    'workgroups:disband',
    async (
      _event,
      { projectPath, groupId }: { projectPath: string; groupId: number }
    ): Promise<{ ok: boolean; error?: string }> => {
      if (!lgrassPath) return { ok: false, error: 'lgrass is not installed' }
      try {
        await runLgrass(lgrassPath, projectPath, ['disband', String(groupId)])
        return { ok: true }
      } catch (err) {
        return { ok: false, error: err instanceof Error ? err.message : String(err) }
      }
    }
  )

  // Called only after the human approves in the join modal. The token is the human's approval, which lgrass redeems through the bridge's claim op.
  ipcMain.handle(
    'workgroups:join',
    async (_event, request: WorkgroupJoinRequest): Promise<WorkgroupJoinResult> => {
      if (!lgrassPath) return { ok: false, error: 'lgrass is not installed' }
      const { projectPath, groupId, tabId } = request
      if (typeof projectPath !== 'string' || !projectPath) return { ok: false, error: 'no project' }
      if (!Number.isSafeInteger(groupId) || groupId < 1)
        return { ok: false, error: 'the workgroup id is not valid' }
      if (!UUID.test(tabId)) return { ok: false, error: 'the tab id is not valid' }
      const token = mintJoinToken({ projectPath, groupId, tabId })
      let threadId: string
      try {
        const out = JSON.parse(
          await runLgrass(lgrassPath, projectPath, ['join', '--token', token, '--json'])
        )
        threadId = String(out.thread_id ?? '')
      } catch (err) {
        return { ok: false, error: err instanceof Error ? err.message : String(err) }
      }
      if (!THREAD_ID.test(threadId)) return { ok: false, error: 'lgrass returned no thread id' }
      const typed = await typeSystemLine(
        tabId,
        `[lg] system: you've been added to workgroup ${groupId}. Introduce yourself briefly in the thread ${threadId}.`
      )
      return typed
        ? { ok: true }
        : { ok: true, warning: 'The tab joined, but the welcome line could not be typed into it.' }
    }
  )
}

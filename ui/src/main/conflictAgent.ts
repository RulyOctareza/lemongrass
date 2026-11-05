// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { execFile } from 'child_process'
import { BrowserWindow, ipcMain } from 'electron'
import type {
  ConflictActionResult,
  ConflictHumanMessage,
  ConflictReviewFile,
  ConflictStartResult,
  ConflictSubmitted,
  ConflictThreadStatus,
  GitConflictOperation,
  GitLineRange
} from '../preload/types'
import { readIntent } from './conflictIntent'
import {
  claimConflictToken,
  claimReply,
  mintConflictToken,
  type ConflictGrant
} from './conflictTokens'
import type { GitRunner } from './gitMove'
import { conflictBlockRanges, conflictOperation, sideRefs } from './gitConflict'
import { WRITE_TIMEOUT_MS, isInside, readEnv, repoRoot, run } from './gitRun'
import { composeShow, composeStartLine } from './reviewCompose'
import { findShellByTab } from './pty'
import { pastePrompt, type PasteOutcome } from './tabNudge'
import { tabStates } from './tabSessions'

// The agent path for conflicts: the app.sock ops `lgrass git conflict` calls from the picked tab,
// and the IPC the resolver calls for the human. Go keeps the thread; every write the human makes
// goes through a token minted here and redeemed by `lgrass git conflict app`.

const LGRASS_TIMEOUT_MS = 10_000
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const THREAD_ID = /^[a-z0-9]{6}$/
// The thread layer's own caps; refused here so a token is never minted for a post lgrass would refuse.
const MAX_BODY = 2000
const MAX_MESSAGES = 50
const MAX_QUOTE = 8000
const MAX_SUBMITTED_FILES = 1000

const OUTCOME_REASONS: Record<Exclude<PasteOutcome, 'typed'>, string> = {
  closed: 'The tab is closed',
  typing: 'You are typing in that tab',
  empty: 'Nothing to send'
}

export interface ConflictOpRequest {
  op: string
  token?: unknown
  project_path?: unknown
  tab_id?: unknown
  scope?: unknown
  thread_id?: unknown
  files?: unknown
}

export interface ConflictOpReply {
  ok: boolean
  error?: string
  text?: string
  conflict?: Record<string, unknown>
}

interface CurrentConflict {
  root: string
  operation: GitConflictOperation
  leftRef: string
  rightRef: string
  scope: string
  paths: string[]
}

let lgrassPath: string | null = null
let windowOf: () => BrowserWindow | undefined = () => undefined

// What the human resolved in the resolver before handing off or sending, by repo root, so `show`
// can tell the model to keep those blocks. In memory only: a restart drops the hint, not the thread.
const resolvedByRoot = new Map<string, { scope: string; files: ConflictReviewFile[] }>()

// One reconcile or write per project at a time, so a status poll and a Send cannot both close or
// post against the same thread.
const queues = new Map<string, Promise<unknown>>()

function serial<T>(key: string, work: () => Promise<T>): Promise<T> {
  const next = (queues.get(key) ?? Promise.resolve()).then(work, work)
  queues.set(
    key,
    next.catch(() => undefined)
  )
  return next
}

function runner(root: string): GitRunner {
  return {
    read: (args) => run(root, args, { env: readEnv }),
    write: (args) =>
      run(root, args, { timeout: WRITE_TIMEOUT_MS, env: { GIT_LITERAL_PATHSPECS: '1' } })
  }
}

function runLgrass(projectPath: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    if (!lgrassPath) return reject(new Error('lgrass is not installed'))
    execFile(
      lgrassPath,
      args,
      { cwd: projectPath, encoding: 'utf8', timeout: LGRASS_TIMEOUT_MS, windowsHide: true },
      (failure, stdout, stderr) =>
        failure ? reject(new Error(stderr.trim() || failure.message)) : resolve(stdout)
    )
  })
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// The scope key matches the renderer's conflictScope: a stopped merge or rebase is the conflict
// until it is continued or aborted, even with every file resolved, while a stash apply is one only
// while files are still unmerged.
async function currentConflict(projectPath: string): Promise<CurrentConflict | null> {
  const root = await repoRoot(projectPath)
  if (!root) return null
  const git = runner(root)
  const listed = await git.read(['diff', '--name-only', '-z', '--diff-filter=U'])
  if (listed.code !== 0) return null
  const paths = [...new Set(listed.stdout.split('\0').filter(Boolean))]
  const operation = await conflictOperation(git)
  if (operation === 'stash' && paths.length === 0) return null
  const { leftRef, rightRef } = await sideRefs(git, operation)
  return {
    root,
    operation,
    leftRef,
    rightRef,
    scope: `conflict:${operation}:${leftRef}:${rightRef}`,
    paths
  }
}

function isStatus(value: unknown): value is ConflictThreadStatus {
  const status = value as ConflictThreadStatus
  return (
    typeof status?.threadId === 'string' &&
    THREAD_ID.test(status.threadId) &&
    typeof status.scope === 'string' &&
    typeof status.assignee === 'string'
  )
}

async function readStatus(projectPath: string): Promise<ConflictThreadStatus | null> {
  const parsed: unknown = JSON.parse(
    await runLgrass(projectPath, ['git', 'conflict', 'status', '--json'])
  )
  if (parsed === null) return null
  if (!isStatus(parsed)) throw new Error('lgrass printed a conflict status it should not')
  return { threadId: parsed.threadId, scope: parsed.scope, assignee: parsed.assignee }
}

async function runApp(projectPath: string, grant: ConflictGrant): Promise<Record<string, unknown>> {
  const token = mintConflictToken(grant)
  const out = await runLgrass(projectPath, ['git', 'conflict', 'app', '--token', token, '--json'])
  return JSON.parse(out) ?? {}
}

async function closeThread(projectPath: string, status: ConflictThreadStatus): Promise<void> {
  await runApp(projectPath, {
    action: 'close',
    projectPath,
    scope: status.scope,
    threadId: status.threadId
  })
}

// Closes the open conflict thread once its scope is no longer the project's conflict, and returns
// what is still open. Runs on every status read and after the git calls that can end a conflict,
// so a thread closes even when no resolver is on screen.
async function reconcile(projectPath: string): Promise<ConflictThreadStatus | null> {
  const status = await readStatus(projectPath)
  if (!status) return null
  const current = await currentConflict(projectPath)
  if (current?.scope === status.scope) return status
  await closeThread(projectPath, status)
  if (current) resolvedByRoot.delete(current.root)
  return null
}

// Called by gitSync after continue, abort, apply and pick. A failure is only logged, since the
// next status read tries again.
export function endConflictThreadIfOver(projectPath: string): void {
  if (!lgrassPath) return
  void serial(projectPath, () => reconcile(projectPath)).catch((err) =>
    console.error('conflict thread reconcile failed:', message(err))
  )
}

function isRange(value: unknown): value is GitLineRange {
  const range = value as GitLineRange
  return Number.isInteger(range?.start) && Number.isInteger(range.end)
}

function validResolved(value: unknown): ConflictReviewFile[] | null {
  if (!Array.isArray(value)) return null
  const files: ConflictReviewFile[] = []
  for (const file of value as ConflictReviewFile[]) {
    if (typeof file?.path !== 'string' || !Array.isArray(file.resolved)) return null
    if (!file.resolved.every(isRange)) return null
    files.push({
      path: file.path,
      resolved: file.resolved.map(({ start, end }) => ({ start, end }))
    })
  }
  return files
}

function validMessages(value: unknown): ConflictHumanMessage[] | string {
  if (!Array.isArray(value) || value.length === 0) return 'Nothing to send'
  if (value.length > MAX_MESSAGES) return `At most ${MAX_MESSAGES} messages go in one send`
  const messages: ConflictHumanMessage[] = []
  for (const item of value as ConflictHumanMessage[]) {
    const body = typeof item?.body === 'string' ? item.body.trim() : ''
    if (!body) return 'A message is empty'
    if (body.length > MAX_BODY) return `A message is longer than ${MAX_BODY} characters`
    const anchor = item.anchor
    if (anchor === null || anchor === undefined) {
      messages.push({ body, anchor: null })
      continue
    }
    if (
      typeof anchor.path !== 'string' ||
      !anchor.path ||
      !Number.isInteger(anchor.startLine) ||
      !Number.isInteger(anchor.endLine) ||
      anchor.startLine < 1 ||
      anchor.endLine < anchor.startLine ||
      typeof anchor.quote !== 'string' ||
      anchor.quote.length > MAX_QUOTE
    ) {
      return 'A line comment does not say where it sits'
    }
    const { path, startLine, endLine, quote } = anchor
    messages.push({ body, anchor: { path, startLine, endLine, quote } })
  }
  return messages
}

function remember(conflict: CurrentConflict, resolved: ConflictReviewFile[]): void {
  const files = resolved.filter((file) => isInside(conflict.root, file.path))
  resolvedByRoot.set(conflict.root, { scope: conflict.scope, files })
}

async function start(
  projectPath: string,
  tabId: string,
  resolvedInput: unknown
): Promise<ConflictStartResult> {
  if (typeof projectPath !== 'string' || !projectPath) return { ok: false, reason: 'No project' }
  if (!UUID.test(tabId)) return { ok: false, reason: 'The tab id is not valid' }
  const resolved = validResolved(resolvedInput)
  if (!resolved) return { ok: false, reason: 'The resolved blocks are not valid' }
  if (!lgrassPath) return { ok: false, reason: 'lgrass is not installed' }
  const state = (await tabStates(projectPath))[tabId]
  if (state?.busy) return { ok: false, reason: state.reason ?? 'The tab is busy' }
  const shell = findShellByTab(tabId)
  if (!shell) return { ok: false, reason: OUTCOME_REASONS.closed }
  if (shell.humanIsTyping()) return { ok: false, reason: OUTCOME_REASONS.typing }

  return serial(projectPath, async (): Promise<ConflictStartResult> => {
    const conflict = await currentConflict(projectPath)
    if (!conflict) return { ok: false, reason: 'No conflict is left in this project' }
    let threadId: string
    try {
      await reconcile(projectPath)
      const out = await runApp(projectPath, {
        action: 'start',
        projectPath,
        scope: conflict.scope,
        tabId
      })
      threadId = String(out.threadId ?? '')
    } catch (err) {
      return { ok: false, reason: message(err) }
    }
    if (!THREAD_ID.test(threadId)) return { ok: false, reason: 'lgrass returned no thread id' }
    remember(conflict, resolved)
    const outcome = await pastePrompt(tabId, composeStartLine(conflict))
    return outcome === 'typed'
      ? { ok: true, threadId }
      : { ok: false, reason: OUTCOME_REASONS[outcome] }
  })
}

async function post(
  projectPath: string,
  messagesInput: unknown,
  resolvedInput: unknown
): Promise<ConflictActionResult> {
  if (typeof projectPath !== 'string' || !projectPath) return { ok: false, reason: 'No project' }
  const messages = validMessages(messagesInput)
  if (typeof messages === 'string') return { ok: false, reason: messages }
  const resolved = validResolved(resolvedInput)
  if (!resolved) return { ok: false, reason: 'The resolved blocks are not valid' }
  if (!lgrassPath) return { ok: false, reason: 'lgrass is not installed' }

  return serial(projectPath, async (): Promise<ConflictActionResult> => {
    try {
      const status = await reconcile(projectPath)
      if (!status) return { ok: false, reason: 'No conflict thread is open' }
      const conflict = await currentConflict(projectPath)
      if (conflict) remember(conflict, resolved)
      await runApp(projectPath, { action: 'post', projectPath, scope: status.scope, messages })
      return { ok: true }
    } catch (err) {
      return { ok: false, reason: message(err) }
    }
  })
}

async function close(projectPath: string): Promise<ConflictActionResult> {
  if (typeof projectPath !== 'string' || !projectPath) return { ok: false, reason: 'No project' }
  if (!lgrassPath) return { ok: false, reason: 'lgrass is not installed' }
  return serial(projectPath, async (): Promise<ConflictActionResult> => {
    try {
      const status = await readStatus(projectPath)
      if (status) await closeThread(projectPath, status)
      return { ok: true }
    } catch (err) {
      return { ok: false, reason: message(err) }
    }
  })
}

let statusFailureLogged = false

async function status(projectPath: string): Promise<ConflictThreadStatus | null> {
  if (typeof projectPath !== 'string' || !projectPath || !lgrassPath) return null
  try {
    return await serial(projectPath, () => reconcile(projectPath))
  } catch (err) {
    // The resolver polls this, so a lasting failure is logged once.
    if (!statusFailureLogged) console.error('lgrass git conflict status failed:', message(err))
    statusFailureLogged = true
    return null
  }
}

// The scope Go sends is the open thread's; refusing when it is not the project's conflict any more
// keeps a model from resolving against an operation that already ended or changed.
async function sameConflict(request: ConflictOpRequest): Promise<CurrentConflict | string> {
  if (typeof request.project_path !== 'string' || !request.project_path) return 'no project path'
  if (!UUID.test(String(request.tab_id ?? ''))) return 'the tab id is not valid'
  if (typeof request.scope !== 'string' || !request.scope) return 'no conflict scope'
  const conflict = await currentConflict(request.project_path)
  if (!conflict) return 'no conflict is left in this project'
  if (conflict.scope !== request.scope)
    return 'the conflict this thread was opened for has ended; ask the human to hand it over again'
  return conflict
}

async function handleShow(request: ConflictOpRequest): Promise<ConflictOpReply> {
  const threadId = String(request.thread_id ?? '')
  if (!THREAD_ID.test(threadId)) return { ok: false, error: 'the thread id is not valid' }
  const conflict = await sameConflict(request)
  if (typeof conflict === 'string') return { ok: false, error: conflict }
  const remembered = resolvedByRoot.get(conflict.root)
  const resolved = new Map(
    remembered?.scope === conflict.scope
      ? remembered.files.map((file) => [file.path, file.resolved])
      : []
  )
  const intent = await readIntent(runner(conflict.root), conflict.operation, conflict.paths)
  const text = composeShow({
    operation: conflict.operation,
    leftRef: conflict.leftRef,
    rightRef: conflict.rightRef,
    threadId,
    files: conflict.paths.map((path) => ({
      path,
      unresolved: conflictBlockRanges(conflict.root, path),
      resolved: resolved.get(path) ?? []
    })),
    intent
  })
  return { ok: true, text }
}

async function handleSubmit(request: ConflictOpRequest): Promise<ConflictOpReply> {
  const conflict = await sameConflict(request)
  if (typeof conflict === 'string') return { ok: false, error: conflict }
  const files = request.files
  if (
    !Array.isArray(files) ||
    files.length > MAX_SUBMITTED_FILES ||
    !files.every((file) => typeof file === 'string' && file && isInside(conflict.root, file))
  ) {
    return { ok: false, error: 'the submitted files are not valid' }
  }
  const window = windowOf()
  if (!window || window.isDestroyed())
    return { ok: false, error: 'the lemongrass window is not open' }
  const submitted: ConflictSubmitted = {
    projectPath: String(request.project_path),
    tabId: String(request.tab_id),
    files: files as string[]
  }
  window.webContents.send('conflict:submitted', submitted)
  return { ok: true }
}

function handleClaim(token: string): ConflictOpReply {
  const grant = claimConflictToken(token)
  if (!grant) return { ok: false, error: 'the conflict token is unknown or has expired' }
  return { ok: true, conflict: claimReply(grant) }
}

export function isConflictOp(op: unknown): boolean {
  return op === 'conflict_show' || op === 'conflict_submit' || op === 'conflict_claim'
}

export async function handleConflictOp(request: ConflictOpRequest): Promise<ConflictOpReply> {
  if (request.op === 'conflict_claim') return handleClaim(String(request.token ?? ''))
  if (request.op === 'conflict_show') return handleShow(request)
  if (request.op === 'conflict_submit') return handleSubmit(request)
  return { ok: false, error: `unknown op ${String(request.op)}` }
}

export function registerConflictAgentHandlers(
  lgrass: string | null,
  getWindow: () => BrowserWindow | undefined
): void {
  lgrassPath = lgrass
  windowOf = getWindow
  ipcMain.handle('conflict:status', (_event, projectPath: string) => status(projectPath))
  ipcMain.handle(
    'conflict:start',
    (_event, projectPath: string, tabId: string, resolved: unknown) =>
      start(projectPath, tabId, resolved)
  )
  ipcMain.handle(
    'conflict:post',
    (_event, projectPath: string, messages: unknown, resolved: unknown) =>
      post(projectPath, messages, resolved)
  )
  ipcMain.handle('conflict:close', (_event, projectPath: string) => close(projectPath))
}

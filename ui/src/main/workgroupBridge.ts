// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { BrowserWindow, dialog, ipcMain } from 'electron'
import { chmodSync, mkdirSync, rmSync, writeFileSync } from 'fs'
import * as net from 'net'
import { homedir } from 'os'
import { join } from 'path'
import { randomUUID } from 'crypto'
import type { WorkgroupSpawnMember, WorkgroupSpawnResult } from '../preload/types'
import { handleNudge } from './tabNudge'
import { handleWebOp, registerWebHandlers } from './web'
import {
  registerApprovalHandlers,
  requestApproval,
  type ApprovalAnswer
} from './workgroupApprovals'

// The socket `lgrass workgroup create` talks to. A proposal waits in the app for the human and,
// once approved, returns a one-time token. Only that token spawns tabs, and it spawns exactly what was shown.

// The composed prompt is lgrass's role header, up to about 1300 characters with the longest names and skills, plus the config's 3000 character assignment.
const MAX_PROMPT_CHARS = 5000
const TOKEN_TTL_MS = 5 * 60_000
const SPAWN_ACK_TIMEOUT_MS = 15_000
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const LABEL = /^[A-Za-z0-9][A-Za-z0-9_-]{0,29}$/
const MODEL = /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,79}$/
const SKILL = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/
const MAX_SKILLS = 9
const VENDORS = new Set(['claude', 'codex'])

interface Proposal {
  op: string
  project_path: string
  leader_tab_id: string
  leader_label: string
  group_name: string
  // Set when the members join a live group instead of starting one.
  group_id?: number
  existing_members?: string[]
  members: {
    tab_id: string
    label: string
    vendor: string
    model?: string
    prompt: string
    skills: string[]
  }[]
  token?: string
  tab_id?: string
  // The `web` op's fields.
  channel_id?: string
  command?: string
  args?: string[]
  flags?: Record<string, string>
}

interface Approved {
  projectPath: string
  leaderTabId: string
  accountsChosen: boolean
  members: WorkgroupSpawnMember[]
  expires: number
}

interface Reply {
  ok: boolean
  approved?: boolean
  token?: string
  error?: string
  typed?: boolean
  reason?: string
  withdrawn?: boolean
  join?: { project_path: string; group_id: number; tab_id: string }
  text?: string
  data?: string
}

const approvals = new Map<string, Approved>()

export interface JoinGrant {
  projectPath: string
  groupId: number
  tabId: string
}

const JOIN_TTL_MS = 60_000
const joins = new Map<string, JoinGrant & { expires: number }>()

// Minted only when the human approves a join in the app, and redeemed once by `lgrass workgroup join` through the claim op.
export function mintJoinToken(grant: JoinGrant): string {
  const now = Date.now()
  for (const [token, join] of joins) if (join.expires < now) joins.delete(token)
  const token = randomUUID()
  joins.set(token, { ...grant, expires: now + JOIN_TTL_MS })
  return token
}

function handleClaim(token: string): Reply {
  const join = joins.get(token)
  joins.delete(token)
  if (!join || join.expires < Date.now())
    return { ok: false, error: 'the join token is unknown or has expired' }
  return {
    ok: true,
    join: {
      project_path: join.projectPath,
      group_id: join.groupId,
      tab_id: join.tabId
    }
  }
}
const spawnWaiters = new Map<string, (result: WorkgroupSpawnResult) => void>()
let dialogQueue: Promise<unknown> = Promise.resolve()

export function workgroupSocketPath(): string {
  return join(homedir(), '.lemongrass', 'app.sock')
}

function validate(p: Proposal): string | null {
  if (typeof p.project_path !== 'string' || !p.project_path) return 'no project path'
  if (!UUID.test(p.leader_tab_id ?? '')) return 'the leader tab id is not valid'
  if (!LABEL.test(p.leader_label ?? '')) return 'the leader label is not valid'
  if (typeof p.group_name !== 'string' || !p.group_name) return 'no group name'
  if (!Array.isArray(p.members) || p.members.length < 1)
    return 'a proposal needs at least one thinker'
  if (p.group_id !== undefined) {
    if (!Number.isSafeInteger(p.group_id) || p.group_id < 1) return 'the workgroup id is not valid'
    if (
      !Array.isArray(p.existing_members) ||
      p.existing_members.length < 1 ||
      !p.existing_members.every((label) => typeof label === 'string' && LABEL.test(label))
    ) {
      return 'the current member list is not valid'
    }
  }
  for (const m of p.members) {
    if (!UUID.test(m.tab_id ?? '')) return 'a thinker tab id is not valid'
    if (!LABEL.test(m.label ?? '')) return 'a thinker label is not valid'
    if (!VENDORS.has(m.vendor)) return 'a thinker vendor is not supported'
    if (m.model && !MODEL.test(m.model)) return 'a thinker model is not valid'
    if (
      !Array.isArray(m.skills) ||
      m.skills.length > MAX_SKILLS ||
      !m.skills.every((name) => typeof name === 'string' && SKILL.test(name))
    ) {
      return 'a thinker skill list is not valid'
    }
    if (typeof m.prompt !== 'string' || !m.prompt || m.prompt.length > MAX_PROMPT_CHARS) {
      return `a thinker prompt must be 1 to ${MAX_PROMPT_CHARS} characters`
    }
  }
  return null
}

function proposalText(p: Proposal): string {
  return p.members
    .map((m) => {
      const model = m.model ? `, model ${m.model}` : ''
      const skills = m.skills.length ? `Must load: ${m.skills.join(', ')}\n` : ''
      return `${m.label} (${m.vendor}${model})\n${skills}${m.prompt}`
    })
    .join('\n\n----\n\n')
}

function writeProposalFile(p: Proposal): string | null {
  const dir = join(homedir(), '.lemongrass', 'proposals')
  const path = join(dir, `${randomUUID()}.txt`)
  try {
    mkdirSync(dir, { recursive: true, mode: 0o700 })
    writeFileSync(path, proposalText(p), { mode: 0o600 })
    return path
  } catch {
    return null
  }
}

function describe(p: Proposal, file: string | null): { message: string; detail: string } {
  return {
    message: describeAsk(p),
    detail: file ? `Full text: ${file}` : 'The full text could not be written to a file.'
  }
}

function describeAsk(p: Proposal): string {
  const count = `${p.members.length} thinker${p.members.length === 1 ? '' : 's'}`
  if (p.group_id !== undefined)
    return `The agent in tab "${p.leader_label}" wants to add ${count} to the workgroup "${p.group_name}".`
  return `The agent in tab "${p.leader_label}" wants to start the workgroup "${p.group_name}" with ${count}.`
}

function askNative(p: Proposal): Promise<boolean> {
  const run = (): Promise<boolean> => {
    const file = writeProposalFile(p)
    const { message, detail } = describe(p, file)
    const options: Electron.MessageBoxOptions = {
      type: 'question',
      title: 'Approve workgroup',
      message,
      detail,
      buttons: ['Approve', 'Decline'],
      defaultId: 1,
      cancelId: 1,
      noLink: true
    }
    return dialog
      .showMessageBox(options)
      .then((result) => result.response === 0)
      .finally(() => {
        if (file) rmSync(file, { force: true })
      })
  }
  const answer = dialogQueue.then(run, run)
  dialogQueue = answer.catch(() => undefined)
  return answer
}

function pruneApprovals(): void {
  const now = Date.now()
  for (const [token, approved] of approvals) if (approved.expires < now) approvals.delete(token)
}

async function askHuman(
  p: Proposal,
  getWindow: () => BrowserWindow | undefined,
  signal: AbortSignal
): Promise<ApprovalAnswer> {
  const window = getWindow()
  if (!window || window.isDestroyed()) return { approved: await askNative(p) }
  return requestApproval(
    window,
    {
      projectPath: p.project_path,
      groupName: p.group_name,
      leaderTabId: p.leader_tab_id,
      leaderLabel: p.leader_label,
      ...(p.group_id !== undefined ? { existingMembers: p.existing_members } : {}),
      members: p.members.map((m) => ({
        label: m.label,
        vendor: m.vendor,
        model: m.model || undefined,
        skills: m.skills,
        prompt: m.prompt
      }))
    },
    signal
  )
}

async function handlePropose(
  p: Proposal,
  getWindow: () => BrowserWindow | undefined,
  signal: AbortSignal
): Promise<Reply> {
  const problem = validate(p)
  if (problem) return { ok: false, error: problem }
  console.info(
    'workgroup: diagnostic proposal',
    JSON.stringify({ projectPath: p.project_path, leaderTabId: p.leader_tab_id })
  )
  const answer = await askHuman(p, getWindow, signal)
  if (!answer.approved) {
    return { ok: true, approved: false, reason: answer.reason, withdrawn: answer.withdrawn }
  }
  pruneApprovals()
  const token = randomUUID()
  approvals.set(token, {
    projectPath: p.project_path,
    leaderTabId: p.leader_tab_id,
    accountsChosen: answer.accounts !== undefined,
    members: p.members.map((m, index) => {
      const account = m.vendor === 'claude' ? answer.accounts?.[index] : undefined
      return {
        tabId: m.tab_id,
        label: m.label,
        vendor: m.vendor,
        model: m.model || undefined,
        prompt: m.prompt,
        ...(account ? { account } : {})
      }
    }),
    expires: Date.now() + TOKEN_TTL_MS
  })
  return { ok: true, approved: true, token }
}

async function handleSpawn(
  token: string,
  getWindow: () => BrowserWindow | undefined
): Promise<Reply> {
  pruneApprovals()
  const approved = approvals.get(token)
  if (!approved) return { ok: false, error: 'the approval is unknown or has expired' }
  approvals.delete(token)
  const window = getWindow()
  if (!window || window.isDestroyed())
    return { ok: false, error: 'the lemongrass window is not open' }

  const requestId = randomUUID()
  console.info(
    'workgroup: diagnostic spawn request',
    JSON.stringify({
      requestId,
      projectPath: approved.projectPath,
      leaderTabId: approved.leaderTabId
    })
  )
  const result = await new Promise<WorkgroupSpawnResult>((resolve) => {
    const timer = setTimeout(() => {
      spawnWaiters.delete(requestId)
      console.info('workgroup: diagnostic spawn timeout', JSON.stringify({ requestId }))
      resolve({ ok: false, error: 'the window did not answer in time' })
    }, SPAWN_ACK_TIMEOUT_MS)
    spawnWaiters.set(requestId, (r) => {
      clearTimeout(timer)
      resolve(r)
    })
    window.webContents.send('workgroup:spawn', {
      requestId,
      projectPath: approved.projectPath,
      leaderTabId: approved.leaderTabId,
      accountsChosen: approved.accountsChosen,
      members: approved.members
    })
  })
  return result.ok ? { ok: true } : { ok: false, error: result.error ?? 'the spawn failed' }
}

function serveConnection(conn: net.Socket, getWindow: () => BrowserWindow | undefined): void {
  let buffer = ''
  const gone = new AbortController()
  conn.setEncoding('utf8')
  conn.setTimeout(11 * 60_000, () => conn.destroy())
  conn.on('error', () => conn.destroy())
  conn.on('close', () => gone.abort())
  conn.on('data', (chunk: string) => {
    buffer += chunk
    if (buffer.length > 200_000) return void conn.destroy()
    const end = buffer.indexOf('\n')
    if (end === -1) return
    const line = buffer.slice(0, end)
    buffer = ''
    void respond(line, getWindow, gone.signal).then((reply) =>
      conn.end(JSON.stringify(reply) + '\n')
    )
  })
}

async function respond(
  line: string,
  getWindow: () => BrowserWindow | undefined,
  signal: AbortSignal
): Promise<Reply> {
  let request: Proposal
  try {
    request = JSON.parse(line)
  } catch {
    return { ok: false, error: 'the request is not valid JSON' }
  }
  try {
    if (request.op === 'propose') return await handlePropose(request, getWindow, signal)
    if (request.op === 'spawn') return await handleSpawn(String(request.token ?? ''), getWindow)
    if (request.op === 'claim') return handleClaim(String(request.token ?? ''))
    if (request.op === 'nudge')
      return await handleNudge(nudgeLgrassdPath, String(request.tab_id ?? ''))
    if (request.op === 'web') return await handleWebOp(request)
    return { ok: false, error: `unknown op ${String(request.op)}` }
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
}

let server: net.Server | undefined
let nudgeLgrassdPath: string | null = null

export function startWorkgroupBridge(
  getWindow: () => BrowserWindow | undefined,
  lgrassdPath: string | null
): void {
  nudgeLgrassdPath = lgrassdPath
  registerApprovalHandlers(getWindow)
  registerWebHandlers()
  ipcMain.on(
    'workgroup:spawn-result',
    (_event, payload: WorkgroupSpawnResult & { requestId: string }) => {
      const waiter = spawnWaiters.get(payload.requestId)
      console.info(
        'workgroup: diagnostic spawn acknowledgement',
        JSON.stringify({ requestId: payload.requestId, waiting: !!waiter, ok: payload.ok })
      )
      if (!waiter) return
      spawnWaiters.delete(payload.requestId)
      waiter({ ok: payload.ok, error: payload.error })
    }
  )

  const path = workgroupSocketPath()
  mkdirSync(join(homedir(), '.lemongrass'), { recursive: true, mode: 0o700 })
  rmSync(path, { force: true })
  server = net.createServer((conn) => serveConnection(conn, getWindow))
  server.on('error', (err) => console.error('workgroup bridge:', err))
  server.listen(path, () => {
    try {
      chmodSync(path, 0o600)
    } catch (err) {
      console.error('workgroup bridge: chmod failed:', err)
    }
  })
}

export function stopWorkgroupBridge(): void {
  server?.close()
  server = undefined
  rmSync(workgroupSocketPath(), { force: true })
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app, BrowserWindow, ipcMain } from 'electron'
import { execFile, spawn } from 'child_process'
import { join } from 'path'
import type { DepEntry, DepsReport, DepsState } from '../preload/types'
import {
  appendOutput,
  helperArch,
  helperExitText,
  installUnsupported,
  nextSeen,
  offerBlocked,
  parseReport,
  requiredRegressed,
  stopsAdbServer,
  successText,
  wingetArgs,
  wingetExitText,
  type RequiredStatuses
} from './depsModel'
import { loadUiState, saveUiState } from './uiState'
import { setGitPath } from './gitRun'
import { stopAllDevices } from './device'

// The check runs each tool's version command, and rpm -qf on a cold cache, so it gets more time than a plain lgrassd call.
const CHECK_TIMEOUT_MS = 60_000

// Root-owned under the install folder, which is what the polkit action names by exact path, so no
// other copy of the helper can run without a prompt.
const HELPER = join(
  process.resourcesPath,
  'bin',
  `linux-${helperArch(process.arch) ?? 'amd64'}`,
  'lgrassroot'
)

let lgrassd: string | null = null
let currentWindow: () => BrowserWindow | undefined = () => undefined
let checking: Promise<void> | null = null
let state: DepsState = {
  phase: 'checking',
  report: null,
  message: null,
  installing: null,
  outcomes: {},
  unsupported: installUnsupported(app.isPackaged, process.platform, process.arch),
  setupDue: loadUiState().setupVersion !== app.getVersion()
}

function set(patch: Partial<DepsState>): void {
  state = { ...state, ...patch }
  const window = currentWindow()
  if (window && !window.isDestroyed()) window.webContents.send('deps:state-changed', state)
}

function runCheck(path: string, fresh: boolean): Promise<DepsReport> {
  const args = ['deps', 'check', '--json', ...(fresh ? ['--fresh'] : [])]
  return new Promise((resolve, reject) => {
    execFile(
      path,
      args,
      { encoding: 'utf8', timeout: CHECK_TIMEOUT_MS, windowsHide: true },
      (failure, stdout, stderr) => {
        if (failure) return reject(new Error(stderr.trim() || failure.message))
        const report = parseReport(stdout)
        if (!report)
          return reject(new Error('deps: lgrassd printed a dependency report the app cannot read'))
        resolve(report)
      }
    )
  })
}

// A check already running is joined rather than started twice.
function check(fresh: boolean): Promise<void> {
  if (checking) return checking
  checking = (async () => {
    if (!lgrassd) {
      set({
        phase: 'failed',
        message: 'lgrassd is not installed, so dependencies cannot be checked.'
      })
      return
    }
    set({ phase: 'checking', message: null })
    try {
      const report = await runCheck(lgrassd, fresh)
      const saved = loadUiState()
      const seen = (saved.setupSeen ?? {}) as RequiredStatuses
      const due = state.setupDue || requiredRegressed(seen, report)
      saveUiState({ setupSeen: nextSeen(seen, report, false) })
      setGitPath(report.deps.find((dep) => dep.name === 'git')?.path ?? '')
      set({ phase: 'ready', report, setupDue: due })
    } catch (err) {
      set({ phase: 'failed', message: err instanceof Error ? err.message : String(err) })
    }
  })().finally(() => (checking = null))
  return checking
}

// What the last check reported for one dependency, or null when no check has returned or it does
// not know the name.
export function knownDep(name: string): DepEntry | null {
  return state.report?.deps.find((dep) => dep.name === name) ?? null
}

// Like knownDep, but waits for a check still running, such as the one at startup.
export async function checkedDep(name: string): Promise<DepEntry | null> {
  if (checking) await checking
  return knownDep(name)
}

// The action and package come from the last report in main, never from the renderer, so a renderer
// can only ask for what the check itself offered.
async function install(name: string): Promise<void> {
  if (state.installing || checking) return
  const dep = state.report?.deps.find((entry) => entry.name === name)
  const offer = dep?.offer ?? null
  if (!dep || !offer || offerBlocked(offer, state.unsupported, process.platform)) return

  const outcomes = { ...state.outcomes }
  delete outcomes[name]
  set({ installing: { name, action: offer.action, package: offer.package, lines: [] }, outcomes })

  const winget = offer.route === 'winget'
  if (winget && stopsAdbServer(name)) await stopAdbServer()

  let partial = ''
  let lastError = ''
  let lastLine = ''
  let errorPartial = ''
  const [file, args] = winget
    ? ['winget', wingetArgs(offer)]
    : ['pkexec', [HELPER, offer.action, offer.package]]
  const code = await new Promise<number | null>((resolve) => {
    const child = spawn(file, args, { stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true })
    const take = (chunk: Buffer): void => {
      if (!state.installing) return
      const [lines, rest] = appendOutput(state.installing.lines, partial, chunk.toString())
      partial = rest
      if (lines.length > 0) lastLine = lines[lines.length - 1]
      set({ installing: { ...state.installing, lines } })
    }
    child.stdout.on('data', take)
    child.stderr.on('data', (chunk: Buffer) => {
      take(chunk)
      const [errors, rest] = appendOutput([], errorPartial, chunk.toString())
      errorPartial = rest
      if (errors.length > 0) lastError = errors[errors.length - 1]
    })
    child.once('error', (err) => {
      lastError = `deps: ${file} could not start: ${err.message}`
      resolve(-1)
    })
    child.once('close', (exitCode) => resolve(exitCode))
  })
  if (errorPartial.trim()) lastError = errorPartial.trim()

  set({ installing: null })
  // dnf exits 0 with nothing to do when the repositories have no newer package, so only the fresh
  // check after the run says whether it fixed anything. On Windows that check finds a tool winget just
  // put on PATH, since lgrassd reads PATH from the registry rather than from the app.
  await check(true)
  const after = state.report?.deps.find((entry) => entry.name === name)
  const failure =
    code === -1
      ? lastError
      : winget
        ? wingetExitText(code, name, lastError || lastLine)
        : helperExitText(code, lastError)
  const outcome =
    code === 0 ? successText(offer.action, after, offer.route) : { ok: false, text: failure }
  set({ outcomes: { ...state.outcomes, [name]: outcome } })
}

// Ends the Device pane's mirrors and the adb server, so winget can replace adb.exe. A failure is ignored,
// since no server running is the usual case and winget reports a file still in use itself.
async function stopAdbServer(): Promise<void> {
  stopAllDevices()
  const adb = knownDep('adb')?.path
  if (!adb) return
  await new Promise<void>((resolve) =>
    execFile(adb, ['kill-server'], { timeout: 10_000, windowsHide: true }, () => resolve())
  )
}

function setupDone(): void {
  saveUiState({
    setupVersion: app.getVersion(),
    ...(state.report ? { setupSeen: nextSeen({}, state.report, true) } : {})
  })
  set({ setupDue: false })
}

// The first check of a new install or update ignores the cache, since the layer is about to show its result.
export function registerDepsHandlers(
  lgrassdPath: string | null,
  getWindow: () => BrowserWindow | undefined
): void {
  lgrassd = lgrassdPath
  currentWindow = getWindow
  ipcMain.handle('deps:state', (): DepsState => state)
  ipcMain.handle('deps:check', async (): Promise<DepsState> => {
    await check(true)
    return state
  })
  ipcMain.handle('deps:install', (_event, name: string): void => {
    if (typeof name === 'string') void install(name)
  })
  ipcMain.handle('deps:setupDone', (): void => setupDone())

  void check(state.setupDue)
}

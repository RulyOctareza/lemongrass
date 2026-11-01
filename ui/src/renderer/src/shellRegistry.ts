// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { accountIsDeleted, accountsSettled } from './accounts'
import { isResumableCommand, spawnArgs } from './shellAgents'
import '@xterm/xterm/css/xterm.css'

// Owns each shell's xterm instance and PTY by tab id, so a shell keeps running
// while its tab is hidden, moved to another pane, or its project is not shown.
// Raw bytes only in both directions. PTY is display-only, never a control-signal source.

const TITLE_PUSH_DELAY_MS = 500
const titlePushTimers = new Map<string, ReturnType<typeof setTimeout>>()

function pushTitle(spec: ShellSpec, title: string): void {
  if (!spec.cwd) return
  const cwd = spec.cwd
  clearTimeout(titlePushTimers.get(spec.id))
  titlePushTimers.set(
    spec.id,
    setTimeout(() => {
      titlePushTimers.delete(spec.id)
      void window.api.tabSessions.title(cwd, spec.id, title)
    }, TITLE_PUSH_DELAY_MS)
  )
}

export interface ShellSpec {
  id: string
  command: string
  cwd?: string
  account?: string
  thinker?: boolean
}

interface ShellSession {
  spec: ShellSpec
  term: Terminal
  fit: FitAddon
  host: HTMLDivElement
  opened: boolean
  started: boolean
  disposed: boolean
  shellId?: string
  resumeDeadline?: number
  // The size the PTY was last told, so a refit that lands on the same cell grid sends nothing.
  ptyCols?: number
  ptyRows?: number
  // Behind its pane's active tab. A sized hidden terminal is never fitted, see showShell.
  hidden: boolean
  webgl?: WebglAddon
  webglUnavailable?: boolean
  nudgeTimer?: ReturnType<typeof setTimeout>
  ptyTimer?: ReturnType<typeof setTimeout>
  showFrame?: number
  showTimer?: ReturnType<typeof setTimeout>
}

const sessions = new Map<string, ShellSession>()
// Tabs asked to take focus before their terminal was opened, focused once it opens.
const pendingFocus = new Set<string>()
const byShellId = new Map<string, ShellSession>()
const bufferedData = new Map<string, string[]>()
const earlyExits = new Map<string, number>()
const initialArgs = new Map<string, string[]>()
const resumeAttempts = new Set<string>()
const closedShellIds = new Set<string>()
// An agent that exits with an error this soon after a resume spawn failed to restore its session.
const RESUME_FAILURE_WINDOW_MS = 10_000
// Escape followed by carriage return, which the agent CLI reads as a newline in its input.
const NEWLINE_SEQUENCE = '\x1b\r'
let exitHandler: ((tabId: string) => void) | undefined
let accountHandler: ((tabId: string, account: string | undefined) => void) | undefined
let listening = false

export const shellTitles = reactive<Record<string, string>>({})
export const shellRestoreFailures = reactive<Record<string, { exitCode: number }>>({})
export const shellAccountGone = reactive<Record<string, string>>({})

export function onShellExit(handler: (tabId: string) => void): void {
  exitHandler = handler
}

export function onShellAccountChange(
  handler: (tabId: string, account: string | undefined) => void
): void {
  accountHandler = handler
}

export function setShellArgs(tabId: string, args: string[]): void {
  initialArgs.set(tabId, args)
}

export function setShellResume(tabId: string, args: string[]): void {
  initialArgs.set(tabId, args)
  resumeAttempts.add(tabId)
}

function handleExit(session: ShellSession, exitCode: number): void {
  const failedResume =
    session.resumeDeadline !== undefined && Date.now() < session.resumeDeadline && exitCode !== 0
  session.resumeDeadline = undefined
  if (failedResume) {
    shellRestoreFailures[session.spec.id] = { exitCode }
    return
  }
  exitHandler?.(session.spec.id)
}

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

const FONT_SIZE = 13
const FONT_WAIT_MS = 3000
let fontReady: Promise<unknown> | undefined

// xterm measures its cell once, at open, from whichever face is available then, and never again
// when a font finishes loading. A terminal opened on the fallback face keeps a row count that no
// longer fits once the real one draws. The timeout keeps a font that fails to load from blocking
// every terminal.
export function terminalFontReady(): Promise<unknown> {
  fontReady ??= Promise.race([
    document.fonts.load(`${FONT_SIZE}px ${cssVar('--font-mono')}`),
    new Promise((resolve) => setTimeout(resolve, FONT_WAIT_MS))
  ])
  return fontReady
}

// A zoom changes devicePixelRatio, and xterm re-measures its cells on its own matchMedia listener.
// A ResizeObserver refit can run before that, leaving a row count for the old cell size with no
// later size change to correct it, so every shell refits a frame after the ratio changes.
function watchPixelRatio(): void {
  const query = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
  query.addEventListener(
    'change',
    () => {
      requestAnimationFrame(() => {
        for (const tabId of sessions.keys()) fitShell(tabId)
      })
      watchPixelRatio()
    },
    { once: true }
  )
}

function listen(): void {
  if (listening) return
  listening = true
  watchPixelRatio()
  window.api.pty.onData(({ id, data }) => {
    const session = byShellId.get(id)
    if (session) {
      session.term.write(data)
      return
    }
    bufferedData.set(id, [...(bufferedData.get(id) ?? []), data])
  })
  window.api.pty.onExit(({ id, exitCode }) => {
    if (closedShellIds.delete(id)) return
    const session = byShellId.get(id)
    if (!session) {
      earlyExits.set(id, exitCode)
      return
    }
    byShellId.delete(id)
    session.shellId = undefined
    handleExit(session, exitCode)
  })
}

function createSession(spec: ShellSpec): ShellSession {
  const term = new Terminal({
    fontFamily: cssVar('--font-mono'),
    fontSize: FONT_SIZE,
    fontWeight: 400,
    fontWeightBold: 600,
    cursorStyle: 'bar',
    cursorBlink: false,
    theme: {
      background: cssVar('--color-surface-1'),
      foreground: cssVar('--color-fg-primary'),
      cursor: cssVar('--color-amber'),
      cursorAccent: cssVar('--color-surface-1'),
      selectionBackground: cssVar('--color-amber-muted'),
      black: cssVar('--color-gray-700'),
      red: cssVar('--color-coral-dim'),
      green: cssVar('--color-jade-dim'),
      yellow: cssVar('--color-amber-dim'),
      blue: cssVar('--color-sapphire-dim'),
      magenta: cssVar('--color-violet-dim'),
      cyan: cssVar('--color-teal-dim'),
      white: cssVar('--color-gray-100'),
      brightBlack: cssVar('--color-gray-500'),
      brightRed: cssVar('--color-coral'),
      brightGreen: cssVar('--color-jade'),
      brightYellow: cssVar('--color-amber'),
      brightBlue: cssVar('--color-sapphire'),
      brightMagenta: cssVar('--color-violet'),
      brightCyan: cssVar('--color-teal'),
      brightWhite: cssVar('--color-white')
    }
  })
  const fit = new FitAddon()
  term.loadAddon(fit)
  const host = document.createElement('div')
  host.style.height = '100%'
  host.style.width = '100%'
  const session: ShellSession = {
    spec,
    term,
    fit,
    host,
    opened: false,
    started: false,
    disposed: false,
    hidden: false
  }

  term.onTitleChange((title) => {
    if (!title) return
    shellTitles[spec.id] = title
    pushTitle(spec, title)
  })
  term.onData((data) => {
    if (session.shellId) window.api.pty.write(session.shellId, data)
  })
  term.attachCustomKeyEventHandler((event) => {
    const clipboardKey = event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey
    if (clipboardKey && (event.code === 'KeyC' || event.code === 'KeyV')) {
      // Chromium also fires a native paste for Ctrl+Shift+V, which xterm would paste a second time.
      event.preventDefault()
      if (event.type === 'keydown') {
        if (event.code === 'KeyC') void copySelection(session)
        else void pasteClipboard(session)
      }
      return false
    }
    if (event.key !== 'Enter' || !event.shiftKey) return true
    if (event.type === 'keydown' && session.shellId) {
      window.api.pty.write(session.shellId, NEWLINE_SEQUENCE)
    }
    return false
  })
  return session
}

async function copySelection(session: ShellSession): Promise<void> {
  const text = session.term.getSelection()
  if (text) await navigator.clipboard.writeText(text)
}

// Goes through Terminal.paste so a shell in bracketed paste mode receives the text as one paste.
async function pasteClipboard(session: ShellSession): Promise<void> {
  const text = await navigator.clipboard.readText()
  if (text && !session.disposed) session.term.paste(text)
}

async function start(session: ShellSession): Promise<void> {
  const { spec, term } = session
  if (spec.command === 'claude' && spec.account) {
    await accountsSettled
    if (session.disposed) return
    if (accountIsDeleted(spec.account)) {
      shellAccountGone[spec.id] = spec.account
      return
    }
  }
  const initial = initialArgs.get(spec.id)
  initialArgs.delete(spec.id)
  const args = spawnArgs(spec.command, spec.id, initial)
  if (resumeAttempts.delete(spec.id)) session.resumeDeadline = Date.now() + RESUME_FAILURE_WINDOW_MS
  const { cols, rows } = term
  const { id } = await window.api.pty.spawn({
    command: spec.command,
    args,
    cwd: spec.cwd,
    account: spec.account,
    orientation: spec.thinker ? 'thinker' : 'tab',
    tabId: isResumableCommand(spec.command) ? spec.id : undefined,
    cols,
    rows
  })
  if (session.disposed) {
    window.api.pty.kill(id)
    return
  }
  session.shellId = id
  session.ptyCols = cols
  session.ptyRows = rows
  byShellId.set(id, session)
  // fitShell drops the PTY resize while shellId is unset, so a refit during the spawn would leave
  // the agent drawing for fewer rows than xterm shows.
  sendPtySize(session)
  for (const chunk of bufferedData.get(id) ?? []) term.write(chunk)
  bufferedData.delete(id)
  const earlyExitCode = earlyExits.get(id)
  if (earlyExitCode !== undefined) {
    earlyExits.delete(id)
    byShellId.delete(id)
    session.shellId = undefined
    handleExit(session, earlyExitCode)
  }
}

export function startFreshShell(tabId: string): void {
  const session = sessions.get(tabId)
  if (!session || session.shellId) return
  delete shellRestoreFailures[tabId]
  session.term.reset()
  void start(session)
}

export function closeFailedShell(tabId: string): void {
  delete shellRestoreFailures[tabId]
  delete shellAccountGone[tabId]
  exitHandler?.(tabId)
}

export function continueOnPrimary(tabId: string): void {
  const session = sessions.get(tabId)
  if (!session || session.shellId || !shellAccountGone[tabId]) return
  delete shellAccountGone[tabId]
  session.spec.account = undefined
  accountHandler?.(tabId, undefined)
  void start(session)
}

function sendPtySize(session: ShellSession): void {
  const { cols, rows } = session.term
  if (!session.shellId || (session.ptyCols === cols && session.ptyRows === rows)) return
  session.ptyCols = cols
  session.ptyRows = rows
  window.api.pty.resize(session.shellId, cols, rows)
}

const PTY_RESIZE_INTERVAL_MS = 50

// Every PTY resize makes the agent repaint, and its CPU grows with the number of resizes, so a drag
// tells the PTY at once, then at most every interval, and the final size when it stops.
function throttlePtySize(session: ShellSession): void {
  if (session.ptyTimer !== undefined) return
  sendPtySize(session)
  const tick = (): void => {
    session.ptyTimer = undefined
    if (session.disposed) return
    const { cols, rows } = session.term
    if (session.ptyCols === cols && session.ptyRows === rows) return
    sendPtySize(session)
    session.ptyTimer = setTimeout(tick, PTY_RESIZE_INTERVAL_MS)
  }
  session.ptyTimer = setTimeout(tick, PTY_RESIZE_INTERVAL_MS)
}

type RenderInternals = {
  _core?: { _renderService?: { _renderDebouncer?: { _innerRefresh?: () => void } } }
}

// The WebGL renderer clears its canvas on resize and redraws on xterm's next animation frame, so a
// resize would paint one blank frame. This runs that redraw now. It reaches into xterm 6 internals,
// as the fit addon does, and if they move it only brings the blank frame back.
function renderNow(term: Terminal): void {
  ;(term as unknown as RenderInternals)._core?._renderService?._renderDebouncer?._innerRefresh?.()
}

// Fits and reports whether the cell grid changed.
function fitTerm(session: ShellSession): boolean {
  const { cols, rows } = session.term
  session.fit.fit()
  shellSized[session.spec.id] = true
  const changed = session.term.cols !== cols || session.term.rows !== rows
  if (changed) renderNow(session.term)
  return changed
}

function fitsHost(session: ShellSession): boolean {
  return session.host.clientWidth > 0 && session.host.clientHeight > 0
}

// Terminals fitted at least once. WorkspacePane keeps a hidden shell laid out only until then, so a
// tab restored into the background spawns its agent at the pane's size.
export const shellSized = reactive<Record<string, boolean>>({})

// A hidden terminal that already has a size is skipped: resizing it would make its agent repaint
// for a tab nobody sees. It catches up in showShell.
export function fitShell(tabId: string): void {
  const session = sessions.get(tabId)
  if (!session || !fitsHost(session) || (session.hidden && shellSized[tabId])) return
  fitTerm(session)
  throttlePtySize(session)
}

const NUDGE_MS = 60

// xterm fills rows gained under an agent that already drew with blanks, and an inline agent
// redraws only its live area when only the height changed. A one-column nudge makes it repaint
// everything. An agent on the alternate screen, such as Claude, repaints on any resize. The two
// sizes go out apart because an agent coalesces resizes sent back to back into none.
function showShell(session: ShellSession): void {
  if (!fitsHost(session)) return
  const { cols, rows } = session.term
  fitTerm(session)
  const term = session.term
  const rowsOnlyGrowth = term.cols === cols && term.rows > rows
  if (session.shellId && rowsOnlyGrowth && term.buffer.active.type === 'normal' && cols > 1) {
    session.ptyCols = cols - 1
    session.ptyRows = term.rows
    window.api.pty.resize(session.shellId, cols - 1, term.rows)
    clearTimeout(session.nudgeTimer)
    session.nudgeTimer = setTimeout(() => sendPtySize(session), NUDGE_MS)
    return
  }
  sendPtySize(session)
}

const WEBGL_LIMIT = 10
// Terminals holding a WebGL renderer, least recently shown first.
const webglRecent: ShellSession[] = []

// WebGL draws on the GPU. A hidden terminal keeps its renderer, which costs nothing while it is
// display: none, so showing it again does not rebuild it. Only the most recently shown ones keep
// one, under Chromium's cap on live WebGL contexts. Without WebGL the DOM renderer draws.
function useWebgl(session: ShellSession): void {
  if (!session.opened || session.webglUnavailable) return
  const at = webglRecent.indexOf(session)
  if (at >= 0) webglRecent.splice(at, 1)
  if (!session.webgl) {
    try {
      const webgl = new WebglAddon()
      webgl.onContextLoss(() => dropWebgl(session))
      session.term.loadAddon(webgl)
      session.webgl = webgl
    } catch {
      session.webglUnavailable = true
      return
    }
  }
  webglRecent.push(session)
  while (webglRecent.length > WEBGL_LIMIT) {
    const oldest = webglRecent.find((other) => other.hidden)
    if (!oldest) break
    dropWebgl(oldest)
  }
}

function dropWebgl(session: ShellSession): void {
  session.webgl?.dispose()
  session.webgl = undefined
  const at = webglRecent.indexOf(session)
  if (at >= 0) webglRecent.splice(at, 1)
}

function cancelShow(session: ShellSession): void {
  if (session.showFrame !== undefined) cancelAnimationFrame(session.showFrame)
  clearTimeout(session.showTimer)
  session.showFrame = undefined
  session.showTimer = undefined
}

// A shown tab paints first and its terminal catches up right after, so a switch never waits on a
// fit or a renderer. The terminal counts as hidden until then, so a ResizeObserver callback in
// between does not fit it ahead of showShell.
export function setShellActive(tabId: string, active: boolean): void {
  const session = sessions.get(tabId)
  if (!session) return
  if (!active) {
    cancelShow(session)
    session.hidden = true
    return
  }
  if (!session.hidden) return useWebgl(session)
  if (session.showFrame !== undefined || session.showTimer !== undefined) return
  session.showFrame = requestAnimationFrame(() => {
    session.showFrame = undefined
    session.showTimer = setTimeout(() => {
      session.showTimer = undefined
      if (session.disposed) return
      session.hidden = false
      useWebgl(session)
      showShell(session)
    })
  })
}

export function shellHasSelection(tabId: string): boolean {
  return sessions.get(tabId)?.term.hasSelection() ?? false
}

export async function copyShellSelection(tabId: string): Promise<void> {
  const session = sessions.get(tabId)
  if (session) await copySelection(session)
}

export async function pasteIntoShell(tabId: string): Promise<void> {
  const session = sessions.get(tabId)
  if (session) await pasteClipboard(session)
}

export function focusShell(tabId: string): void {
  const session = sessions.get(tabId)
  if (session?.opened) session.term.focus()
  else pendingFocus.add(tabId)
}

// Starts a shell's process before its tab is shown, because only the active project's tabs are mounted and a workgroup thinker must run whichever project is on screen.
export function startShellDetached(spec: ShellSpec): void {
  listen()
  if (sessions.has(spec.id)) return
  const session = createSession(spec)
  sessions.set(spec.id, session)
  session.started = true
  void start(session)
}

export function attachShell(spec: ShellSpec, container: HTMLElement): void {
  listen()
  let session = sessions.get(spec.id)
  if (!session) {
    session = createSession(spec)
    sessions.set(spec.id, session)
  }
  container.appendChild(session.host)
  if (!session.opened) {
    session.opened = true
    session.term.open(session.host)
    if (pendingFocus.delete(spec.id)) session.term.focus()
    if (session.started) return fitShell(spec.id)
    session.started = true
    session.fit.fit()
    void start(session)
    return
  }
  fitShell(spec.id)
  session.term.refresh(0, session.term.rows - 1)
}

export function disposeShell(tabId: string): void {
  pendingFocus.delete(tabId)
  const session = sessions.get(tabId)
  if (!session) return
  sessions.delete(tabId)
  clearTimeout(titlePushTimers.get(tabId))
  titlePushTimers.delete(tabId)
  delete shellTitles[tabId]
  delete shellRestoreFailures[tabId]
  delete shellAccountGone[tabId]
  session.disposed = true
  clearTimeout(session.nudgeTimer)
  clearTimeout(session.ptyTimer)
  cancelShow(session)
  dropWebgl(session)
  delete shellSized[tabId]
  if (session.shellId) {
    byShellId.delete(session.shellId)
    closedShellIds.add(session.shellId)
    window.api.pty.gracefulClose(session.shellId)
  }
  session.term.dispose()
  session.host.remove()
}

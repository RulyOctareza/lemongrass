import { reactive } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
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
}

const sessions = new Map<string, ShellSession>()
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

function listen(): void {
  if (listening) return
  listening = true
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
    fontSize: 13,
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
    disposed: false
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
  const { id } = await window.api.pty.spawn({
    command: spec.command,
    args,
    cwd: spec.cwd,
    account: spec.account,
    orientation: spec.thinker ? 'thinker' : 'tab',
    tabId: isResumableCommand(spec.command) ? spec.id : undefined,
    cols: term.cols,
    rows: term.rows
  })
  if (session.disposed) {
    window.api.pty.kill(id)
    return
  }
  session.shellId = id
  byShellId.set(id, session)
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

export function fitShell(tabId: string): void {
  const session = sessions.get(tabId)
  if (!session || session.host.clientWidth === 0 || session.host.clientHeight === 0) return
  session.fit.fit()
  if (session.shellId) window.api.pty.resize(session.shellId, session.term.cols, session.term.rows)
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
  sessions.get(tabId)?.term.focus()
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
  const session = sessions.get(tabId)
  if (!session) return
  sessions.delete(tabId)
  clearTimeout(titlePushTimers.get(tabId))
  titlePushTimers.delete(tabId)
  delete shellTitles[tabId]
  delete shellRestoreFailures[tabId]
  delete shellAccountGone[tabId]
  session.disposed = true
  if (session.shellId) {
    byShellId.delete(session.shellId)
    closedShellIds.add(session.shellId)
    window.api.pty.gracefulClose(session.shellId)
  }
  session.term.dispose()
  session.host.remove()
}

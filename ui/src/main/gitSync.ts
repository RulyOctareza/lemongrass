import { ipcMain } from 'electron'
import {
  WRITE_TIMEOUT_MS,
  errorText,
  pushEnv,
  readEnv,
  repoRoot,
  run,
  stoppedOperation
} from './gitRun'
import type { GitActionResult, GitIncomingCommit } from '../preload/types'

const notARepo = 'This project is not a git repository.'
const FETCH_TIMEOUT_MS = 60_000
const MAX_INCOMING = 100
// An unset editor makes git wait on a terminal that is not there.
const noEditor = { GIT_EDITOR: 'true' }

// A target is a branch the repository already has, so nothing else can reach git as a revision.
async function isBranch(root: string, name: string): Promise<boolean> {
  if (!name || name.startsWith('-') || name.includes('\0')) return false
  for (const prefix of ['refs/heads/', 'refs/remotes/']) {
    const found = await run(root, ['show-ref', '--verify', '--quiet', `${prefix}${name}`], {
      env: readEnv
    })
    if (found.code === 0) return true
  }
  return false
}

async function fetchAll(projectPath: string): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const result = await run(root, ['fetch', '--all'], { timeout: FETCH_TIMEOUT_MS, env: pushEnv })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

// The commits the target has that the checked-out branch does not.
async function incoming(projectPath: string, target: string): Promise<GitIncomingCommit[]> {
  const root = await repoRoot(projectPath)
  if (!root || !(await isBranch(root, target))) return []
  const result = await run(
    root,
    ['log', `HEAD..${target}`, `-n${MAX_INCOMING}`, '--format=%H%x1f%an%x1f%ct%x1f%s'],
    { env: readEnv }
  )
  if (result.code !== 0) return []
  return result.stdout
    .split('\n')
    .filter(Boolean)
    .map((line) => {
      const [hash, author, time, subject] = line.split('\x1f')
      return { hash, author, time: Number(time), subject }
    })
}

async function integrate(
  projectPath: string,
  target: string,
  mode: 'merge' | 'rebase'
): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  if (!(await isBranch(root, target))) return { ok: false, error: 'That is not a branch.' }
  if (await stoppedOperation(root)) {
    return { ok: false, error: 'A merge or rebase is already in progress.' }
  }
  const args = mode === 'merge' ? ['merge', '--no-edit', target] : ['rebase', target]
  const result = await run(root, args, { timeout: WRITE_TIMEOUT_MS, env: noEditor })
  // A stop on conflicts exits non-zero too, and the panel then offers Continue and Abort.
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

async function finish(projectPath: string, action: 'continue' | 'abort'): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const operation = await stoppedOperation(root)
  if (!operation) return { ok: false, error: 'No merge or rebase is in progress.' }
  const result = await run(root, [operation, `--${action}`], {
    timeout: WRITE_TIMEOUT_MS,
    env: noEditor
  })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

export function registerGitSyncHandlers(): void {
  ipcMain.handle('git:fetch', (_event, projectPath: string) => fetchAll(projectPath))
  ipcMain.handle('git:incoming', (_event, projectPath: string, target: string) =>
    incoming(projectPath, target)
  )
  ipcMain.handle(
    'git:integrate',
    (_event, projectPath: string, target: string, mode: 'merge' | 'rebase') => {
      if (mode !== 'merge' && mode !== 'rebase') return { ok: false, error: 'Unknown mode.' }
      return integrate(projectPath, target, mode)
    }
  )
  ipcMain.handle('git:finish', (_event, projectPath: string, action: 'continue' | 'abort') => {
    if (action !== 'continue' && action !== 'abort') return { ok: false, error: 'Unknown action.' }
    return finish(projectPath, action)
  })
}

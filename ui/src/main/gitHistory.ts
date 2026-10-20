import { ipcMain } from 'electron'
import { parseBlame, parseLog, type RefNames } from './gitParse'
import { WRITE_TIMEOUT_MS, errorText, readEnv, repoRoot, run } from './gitRun'
import type {
  GitActionResult,
  GitBlame,
  GitCommitInfo,
  GitLogResult,
  GitLogSearchMode,
  GitResetPreview
} from '../preload/types'

const notARepo = 'This project is not a git repository.'
const COMMIT_HASH = /^[0-9a-f]{4,64}$/
const MAX_BLAME_BYTES = 4 * 1024 * 1024
const MAX_PAGE = 500
const LOG_FORMAT = '--format=%H%x1f%P%x1f%an%x1f%ct%x1f%D%x1f%s%x1e'

async function refNames(root: string): Promise<RefNames> {
  const names: RefNames = { local: new Set(), remote: new Set() }
  const result = await run(
    root,
    ['for-each-ref', '--format=%(refname)', 'refs/heads', 'refs/remotes'],
    {
      env: readEnv
    }
  )
  for (const ref of result.stdout.split('\n').filter(Boolean)) {
    if (ref.startsWith('refs/heads/')) names.local.add(ref.slice('refs/heads/'.length))
    else if (!ref.endsWith('/HEAD')) names.remote.add(ref.slice('refs/remotes/'.length))
  }
  return names
}

// Commits no remote branch contains. With no remote at all that is every commit.
async function unpushedHashes(root: string): Promise<Set<string>> {
  const result = await run(
    root,
    ['rev-list', '--all', '--exclude=refs/stash', '--not', '--remotes'],
    {
      env: readEnv
    }
  )
  return new Set(result.code === 0 ? result.stdout.split('\n').filter(Boolean) : [])
}

// Search values reach git only through `--opt=value` or after `--`, so none can act as an option.
function searchArgs(mode: GitLogSearchMode, text: string): string[] | null {
  if (mode === 'message') return ['-i', '--fixed-strings', `--grep=${text}`]
  if (mode === 'author') return ['-i', '--fixed-strings', `--author=${text}`]
  if (mode === 'file') return ['--', text]
  return null
}

async function log(
  projectPath: string,
  mode: GitLogSearchMode,
  text: string,
  skip: number,
  limit: number
): Promise<GitLogResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { status: 'error', message: notARepo }
  const needle = text.trim()
  const count = Math.min(Math.max(limit, 1), MAX_PAGE)
  const from = Math.max(skip, 0)

  let scope = ['--exclude=refs/stash', '--all']
  let filter: string[] = []
  if (needle && mode === 'hash') {
    if (!COMMIT_HASH.test(needle)) return { status: 'ok', commits: [], hasMore: false, head: null }
    const found = await run(root, ['rev-parse', '--verify', '--quiet', `${needle}^{commit}`], {
      env: readEnv
    })
    if (found.code !== 0) return { status: 'ok', commits: [], hasMore: false, head: null }
    scope = ['--no-walk', found.stdout.trim()]
  } else if (needle) {
    filter = searchArgs(mode, needle) ?? []
  }

  const [names, unpushed, head, result] = await Promise.all([
    refNames(root),
    unpushedHashes(root),
    run(root, ['rev-parse', '--verify', '--quiet', 'HEAD'], { env: readEnv }),
    run(
      root,
      ['log', '--topo-order', `-n${count + 1}`, `--skip=${from}`, LOG_FORMAT, ...scope, ...filter],
      { env: readEnv }
    )
  ])
  // A repository with no commits makes log fail, and that is an empty history, not an error.
  if (result.code !== 0 && head.code !== 0) {
    return { status: 'ok', commits: [], hasMore: false, head: null }
  }
  if (result.code !== 0) return { status: 'error', message: errorText(result) }
  const commits = parseLog(result.stdout, names, unpushed)
  return {
    status: 'ok',
    commits: commits.slice(0, count),
    hasMore: commits.length > count,
    head: head.code === 0 ? head.stdout.trim() : null
  }
}

// Changes whenever a branch, a remote branch, a tag or HEAD moves, so the tab can reload on it.
async function logToken(projectPath: string): Promise<string> {
  const root = await repoRoot(projectPath)
  if (!root) return ''
  const refs = await run(
    root,
    [
      'for-each-ref',
      '--format=%(objectname) %(refname)',
      'refs/heads',
      'refs/remotes',
      'refs/tags'
    ],
    { env: readEnv }
  )
  const head = await run(root, ['rev-parse', 'HEAD'], { env: readEnv })
  return `${head.stdout.trim()}\n${refs.stdout}`
}

async function commitInfo(projectPath: string, hash: string): Promise<GitCommitInfo | null> {
  const root = await repoRoot(projectPath)
  if (!root || !COMMIT_HASH.test(hash)) return null
  const result = await run(
    root,
    ['show', '-s', '--format=%H%x1f%P%x1f%an%x1f%ae%x1f%ct%x1f%B', hash],
    { env: readEnv }
  )
  if (result.code !== 0) return null
  const [full, parents, author, email, time, ...message] = result.stdout.split('\x1f')
  return {
    hash: full,
    parents: parents ? parents.split(' ') : [],
    author,
    email,
    time: Number(time),
    message: message.join('\x1f').trim()
  }
}

async function blame(projectPath: string, hash: string, path: string): Promise<GitBlame> {
  const root = await repoRoot(projectPath)
  if (!root) return { status: 'error', message: notARepo }
  if (!COMMIT_HASH.test(hash)) return { status: 'error', message: 'That is not a valid commit.' }
  if (!path || path.includes('\0')) return { status: 'error', message: 'That is not a valid path.' }
  const result = await run(root, ['blame', '--porcelain', hash, '--', path], {
    env: readEnv,
    maxBuffer: MAX_BLAME_BYTES
  })
  if (result.overflow) return { status: 'too-large' }
  if (result.code !== 0) return { status: 'error', message: errorText(result) }
  return { status: 'ok', lines: parseBlame(result.stdout) }
}

async function resetPreview(projectPath: string, hash: string): Promise<GitResetPreview> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  if (!COMMIT_HASH.test(hash)) return { ok: false, error: 'That is not a valid commit.' }
  const ancestor = await run(root, ['merge-base', '--is-ancestor', hash, 'HEAD'], {
    env: readEnv
  })
  if (ancestor.code !== 0) {
    return { ok: false, error: 'That commit is not in the history of the checked-out branch.' }
  }
  const all = await run(root, ['rev-list', '--count', `${hash}..HEAD`], { env: readEnv })
  const local = await run(root, ['rev-list', '--count', `${hash}..HEAD`, '--not', '--remotes'], {
    env: readEnv
  })
  if (all.code !== 0 || local.code !== 0) return { ok: false, error: errorText(all) }
  const count = Number(all.stdout.trim())
  return { ok: true, count, pushed: count - Number(local.stdout.trim()) }
}

async function resetSoft(projectPath: string, hash: string): Promise<GitActionResult> {
  const preview = await resetPreview(projectPath, hash)
  if (!preview.ok) return { ok: false, error: preview.error }
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const result = await run(root, ['reset', '--soft', hash], { timeout: WRITE_TIMEOUT_MS })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

// --only without paths rewrites the message and leaves whatever is staged out of the commit.
async function amendMessage(projectPath: string, message: string): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const text = message.trim()
  if (!text) return { ok: false, error: 'The commit message is empty.' }
  const result = await run(root, ['commit', '--amend', '--only', '-F', '-'], {
    input: `${text}\n`,
    timeout: WRITE_TIMEOUT_MS
  })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

export function registerGitHistoryHandlers(): void {
  ipcMain.handle(
    'git:log',
    (
      _event,
      projectPath: string,
      mode: GitLogSearchMode,
      text: string,
      skip: number,
      limit: number
    ) => log(projectPath, mode, text, skip, limit)
  )
  ipcMain.handle('git:logToken', (_event, projectPath: string) => logToken(projectPath))
  ipcMain.handle('git:commitInfo', (_event, projectPath: string, hash: string) =>
    commitInfo(projectPath, hash)
  )
  ipcMain.handle('git:blame', (_event, projectPath: string, hash: string, path: string) =>
    blame(projectPath, hash, path)
  )
  ipcMain.handle('git:resetPreview', (_event, projectPath: string, hash: string) =>
    resetPreview(projectPath, hash)
  )
  ipcMain.handle('git:resetSoft', (_event, projectPath: string, hash: string) =>
    resetSoft(projectPath, hash)
  )
  ipcMain.handle('git:amendMessage', (_event, projectPath: string, message: string) =>
    amendMessage(projectPath, message)
  )
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { execFile, type ExecFileException } from 'child_process'
import { existsSync } from 'fs'
import { join, resolve, sep } from 'path'

export const READ_TIMEOUT_MS = 15_000
export const WRITE_TIMEOUT_MS = 120_000
export const MAX_OUTPUT_BYTES = 16 * 1024 * 1024

export interface RunResult {
  code: number | null
  stdout: string
  stderr: string
  overflow: boolean
}

export interface RunOptions {
  input?: string
  timeout?: number
  maxBuffer?: number
  env?: Record<string, string>
}

export function run(cwd: string, args: string[], options: RunOptions = {}): Promise<RunResult> {
  return new Promise((resolvePromise) => {
    const child = execFile(
      'git',
      args,
      {
        cwd,
        encoding: 'utf8',
        timeout: options.timeout ?? READ_TIMEOUT_MS,
        maxBuffer: options.maxBuffer ?? MAX_OUTPUT_BYTES,
        env: { ...process.env, ...options.env }
      },
      (failure: ExecFileException | null, stdout, stderr) => {
        if (!failure) return resolvePromise({ code: 0, stdout, stderr, overflow: false })
        if (failure.code === 'ENOENT') {
          return resolvePromise({
            code: null,
            stdout: '',
            stderr: 'git was not found on this machine.',
            overflow: false
          })
        }
        resolvePromise({
          code: typeof failure.code === 'number' ? failure.code : null,
          stdout,
          stderr: stderr || failure.message,
          overflow: failure.code === 'ERR_CHILD_PROCESS_STDIO_MAXBUFFER'
        })
      }
    )
    if (options.input !== undefined) child.stdin?.end(options.input)
  })
}

export const readEnv = { GIT_OPTIONAL_LOCKS: '0', LC_ALL: 'C', GIT_LITERAL_PATHSPECS: '1' }
export const pushEnv = { GIT_TERMINAL_PROMPT: '0' }

export const topLevelCache = new Map<string, string>()

export async function repoRoot(projectPath: string): Promise<string | null> {
  const cached = topLevelCache.get(projectPath)
  if (cached) return cached
  const result = await run(projectPath, ['rev-parse', '--show-toplevel'], { env: readEnv })
  const root = result.code === 0 ? result.stdout.trim() : ''
  if (!root) return null
  topLevelCache.set(projectPath, root)
  return root
}

export function isInside(root: string, relativePath: string): boolean {
  if (!relativePath || relativePath.includes('\0')) return false
  const target = resolve(root, relativePath)
  return target.startsWith(root + sep)
}

export function errorText(result: RunResult): string {
  return result.stderr.trim() || result.stdout.trim() || 'git failed without a message.'
}

const gitDirCache = new Map<string, string>()

// The stopped merge or rebase is recorded only as files in the git directory.
export async function stoppedOperation(root: string): Promise<'merge' | 'rebase' | null> {
  let gitDir = gitDirCache.get(root)
  if (!gitDir) {
    const result = await run(root, ['rev-parse', '--absolute-git-dir'], { env: readEnv })
    gitDir = result.code === 0 ? result.stdout.trim() : ''
    if (!gitDir) return null
    gitDirCache.set(root, gitDir)
  }
  if (existsSync(join(gitDir, 'rebase-merge')) || existsSync(join(gitDir, 'rebase-apply'))) {
    return 'rebase'
  }
  return existsSync(join(gitDir, 'MERGE_HEAD')) ? 'merge' : null
}

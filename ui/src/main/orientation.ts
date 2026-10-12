import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

export type OrientationRole = 'tab' | 'thinker'

export interface OrientationLaunch {
  args: string[]
  env: Record<string, string>
}

// The SessionStart hook skips the laws summary for a session started with this set.
export const LAWS_IN_PROMPT_ENV = 'LGRASS_LAWS_IN_PROMPT'

function readIfExists(path: string): string | undefined {
  return existsSync(path) ? readFileSync(path, 'utf8') : undefined
}

// A project's laws summary follows the orientation, so tabs of one project share both in the prompt cache.
function promptFile(
  home: string,
  role: OrientationRole,
  orientation: string,
  laws: string,
  cwd: string
): string {
  const dir = join(home, '.lemongrass', 'prompt')
  const key = createHash('sha256').update(cwd).digest('hex').slice(0, 16)
  const path = join(dir, `${key}-${role}.md`)
  const content = `${orientation.trimEnd()}\n\n${laws.trim()}\n`
  if (readIfExists(path) !== content) {
    mkdirSync(dir, { recursive: true })
    writeFileSync(path, content)
  }
  return path
}

// lgrassconf installs one orientation per role; a Claude tab spawned before it exists starts without one.
export function orientationLaunch(
  command: string,
  role: OrientationRole,
  home: string,
  cwd?: string
): OrientationLaunch {
  if (command !== 'claude') return { args: [], env: {} }
  const orientationPath = join(home, '.lemongrass', 'orientation', `${role}.md`)
  const orientation = readIfExists(orientationPath)
  if (orientation === undefined) return { args: [], env: {} }
  const laws = cwd ? readIfExists(join(cwd, 'biblio', 'laws', 'summary.md')) : undefined
  if (laws === undefined || laws.trim() === '' || cwd === undefined) {
    return { args: ['--append-system-prompt-file', orientationPath], env: {} }
  }
  return {
    args: ['--append-system-prompt-file', promptFile(home, role, orientation, laws, cwd)],
    env: { [LAWS_IN_PROMPT_ENV]: '1' }
  }
}

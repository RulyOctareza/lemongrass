import { existsSync } from 'node:fs'
import { join } from 'node:path'

export type OrientationRole = 'tab' | 'thinker'

// lgrassconf installs one orientation per role; a Claude tab spawned before it exists starts without one.
export function orientationArgs(command: string, role: OrientationRole, home: string): string[] {
  if (command !== 'claude') return []
  const path = join(home, '.lemongrass', 'orientation', `${role}.md`)
  return existsSync(path) ? ['--append-system-prompt-file', path] : []
}

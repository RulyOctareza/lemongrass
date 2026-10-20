import { accessSync, constants } from 'node:fs'
import { delimiter, isAbsolute, join } from 'node:path'

function executable(path: string): boolean {
  try {
    accessSync(path, constants.X_OK)
    return true
  } catch {
    return false
  }
}

function onPath(name: string, pathVar: string): string | undefined {
  for (const dir of pathVar.split(delimiter)) {
    if (dir && executable(join(dir, name))) return name
  }
  return undefined
}

// The Shell agent is stored as "zsh" in saved tabs, but a host may not have zsh, and a spawn of a missing binary makes the tab close at once.
export function resolveShellBinary(
  command: string,
  env: Record<string, string | undefined> = process.env
): string {
  if (command !== 'zsh') return command
  const login = env.SHELL
  if (login && isAbsolute(login) && executable(login)) return login
  const pathVar = env.PATH ?? ''
  return onPath('zsh', pathVar) ?? onPath('bash', pathVar) ?? onPath('sh', pathVar) ?? command
}

import { execFile } from 'child_process'

const TRUST_TIMEOUT_MS = 5_000

let lgrassdPath: string | null = null

export function setTrustBinary(path: string | null): void {
  lgrassdPath = path
}

// Resolves on failure too, because a missing trust entry only brings back Claude Code's or Codex's own dialog.
export function writeTrust(projectPath: string, target?: string): Promise<boolean> {
  return new Promise((resolve) => {
    if (!lgrassdPath) return resolve(false)
    execFile(
      lgrassdPath,
      ['trust', projectPath, ...(target ? ['--target', target] : [])],
      { encoding: 'utf8', timeout: TRUST_TIMEOUT_MS },
      (failure, _stdout, stderr) => {
        if (failure) console.error('lgrassd trust failed:', stderr || failure)
        resolve(!failure)
      }
    )
  })
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'

const AUTO_LOCK_MS = 5 * 60 * 1000

export const vaultSession = reactive({
  unlocked: false,
  passphrase: '',
  shortIdByChannel: {} as Record<string, string>,
  shortIdByHTTPChannel: {} as Record<string, string>
})

const knownErrors: Record<string, string> = {
  'vault: wrong passphrase': 'Wrong passphrase.'
}

// Electron wraps a failed ipcRenderer.invoke as "Error invoking remote method '...': Error: <message>".
export function describeError(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err)
  const stripped = raw.replace(/^Error invoking remote method '[^']*': (Error: )?/, '')
  const lockout = stripped.match(
    /^vault: too many failed attempts, locked out(?:, try again in (.+))?$/
  )
  if (lockout) {
    return `Too many wrong passphrase attempts. Try again in ${lockout[1] ?? 'a few minutes'}.`
  }
  return knownErrors[stripped] ?? stripped
}

let idleTimer: ReturnType<typeof setTimeout> | null = null

function clearIdleTimer(): void {
  if (idleTimer) {
    clearTimeout(idleTimer)
    idleTimer = null
  }
}

export function clearShortIds(): void {
  vaultSession.shortIdByChannel = {}
  vaultSession.shortIdByHTTPChannel = {}
}

export function lockVaultSession(): void {
  vaultSession.unlocked = false
  vaultSession.passphrase = ''
  clearIdleTimer()
}

// Every unlock also moves a policy sealed by an earlier version into the readable policy file, which is a no-op once done.
export function unlockVaultSession(passphrase: string): void {
  vaultSession.passphrase = passphrase
  vaultSession.unlocked = true
  noteVaultActivity()
  window.api.vault.migratePolicy(passphrase).catch(() => undefined)
}

export function noteVaultActivity(): void {
  if (!vaultSession.unlocked) return
  clearIdleTimer()
  idleTimer = setTimeout(lockVaultSession, AUTO_LOCK_MS)
}

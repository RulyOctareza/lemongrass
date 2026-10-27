// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { reactive } from 'vue'
import type { AccountInfo } from '../../preload/types'

export const PRIMARY_ACCOUNT_DIR = '~/.claude'

const PREF_KEY = 'lemongrass.claudeAccount'
const NAME_PATTERN = /^[a-z0-9-]{1,32}$/

export const accountStore = reactive<{ items: AccountInfo[]; loadFailed: boolean }>({
  items: [],
  loadFailed: false
})

let markSettled: () => void = () => {}
export const accountsSettled = new Promise<void>((resolve) => {
  markSettled = resolve
})

export async function loadAccounts(): Promise<void> {
  try {
    accountStore.items = await window.api.accounts.list()
    accountStore.loadFailed = false
  } catch {
    accountStore.loadFailed = true
  } finally {
    markSettled()
  }
}

export function accountIsDeleted(name: string): boolean {
  return !accountStore.loadFailed && !accountStore.items.some((account) => account.name === name)
}

export function accountNameError(name: string): string {
  if (!name) return ''
  if (name === 'primary') return 'This name is reserved for the primary account.'
  if (!NAME_PATTERN.test(name)) {
    return 'Use lowercase letters, digits and hyphens, up to 32 characters.'
  }
  if (accountStore.items.some((account) => account.name === name)) {
    return 'An account with this name already exists.'
  }
  return ''
}

export async function addAccount(name: string): Promise<string> {
  const result = await window.api.accounts.add(name)
  await loadAccounts()
  return result.ok ? '' : result.error
}

export async function removeAccount(name: string): Promise<string> {
  const result = await window.api.accounts.remove(name)
  await loadAccounts()
  return result.ok ? '' : result.error
}

export function preferredAccount(): string | undefined {
  try {
    const saved = localStorage.getItem(PREF_KEY)
    if (saved && accountStore.items.some((account) => account.name === saved)) return saved
  } catch {
    // ignore
  }
  return undefined
}

export function setPreferredAccount(name: string | undefined): void {
  try {
    if (name) localStorage.setItem(PREF_KEY, name)
    else localStorage.removeItem(PREF_KEY)
  } catch {
    // ignore
  }
}

export function accountLabel(account: string | undefined): string {
  return account ?? 'primary'
}

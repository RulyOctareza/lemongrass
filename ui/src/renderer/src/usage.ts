// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { AccountUsage, CodexUsage, UsageWindow } from '../../preload/types'

const REFRESH_MS = 60_000

export const usageStore = reactive<{
  claude: Record<string, AccountUsage>
  codex: CodexUsage | null
  loadedAt: number
}>({
  claude: {},
  codex: null,
  loadedAt: 0
})

export async function refreshUsage(): Promise<void> {
  const report = await window.api.usage.get()
  if (!report) return
  const claude: Record<string, AccountUsage> = {}
  for (const account of report.claude) claude[account.account] = account
  usageStore.claude = claude
  usageStore.codex = report.codex
  usageStore.loadedAt = Date.now()
}

// The figures only change while an agent runs, so polling stops while the window is in the background and catches up
// on focus.
export function startUsagePolling(): void {
  void refreshUsage()
  setInterval(() => {
    if (document.hasFocus()) void refreshUsage()
  }, REFRESH_MS)
  window.addEventListener('focus', () => {
    if (Date.now() - usageStore.loadedAt >= REFRESH_MS) void refreshUsage()
  })
}

export function claudeUsage(account: string | undefined): AccountUsage | undefined {
  return usageStore.claude[account ?? 'primary']
}

export function windowsSummary(windows: UsageWindow[]): string {
  return windows.map((w) => `${w.label} ${Math.round(w.used_percent)}%`).join(' · ')
}

export function highestUse(windows: UsageWindow[]): number {
  return windows.reduce((max, w) => Math.max(max, w.used_percent), 0)
}

export function usageLevel(windows: UsageWindow[]): 'normal' | 'warn' | 'danger' {
  const top = highestUse(windows)
  if (top >= 90) return 'danger'
  if (top >= 75) return 'warn'
  return 'normal'
}

function relative(seconds: number): string {
  const abs = Math.abs(seconds)
  if (abs < 60) return 'less than a minute'
  if (abs < 3600) return `${Math.round(abs / 60)} min`
  if (abs < 86400) return `${Math.round(abs / 3600)} h`
  return `${Math.round(abs / 86400)} d`
}

export function resetText(w: UsageWindow, nowMs = Date.now()): string {
  if (w.reset) return `${w.label}: reset, no use reported since.`
  const at = new Date(w.resets_at * 1000)
  const when = at.toLocaleString(undefined, {
    weekday: 'short',
    hour: '2-digit',
    minute: '2-digit'
  })
  return `${w.label}: ${Math.round(w.used_percent)}% used, resets ${when} (in ${relative(w.resets_at - nowMs / 1000)}).`
}

export function seenText(seenAt: number, nowMs = Date.now()): string {
  return `As of ${relative(nowMs / 1000 - seenAt)} ago.`
}

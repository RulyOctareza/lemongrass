// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { AccountUsage, CodexUsage, ContextUsage, UsageWindow } from '../../preload/types'

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

// Each Claude tab's context window use by tab id. The main process pushes a fresh copy whenever a tab's figure
// changes, so this needs no poll.
export const contextStore = reactive<{ tabs: Record<string, ContextUsage> }>({ tabs: {} })

export function startContextFeed(): void {
  window.api.usage.onContexts((tabs) => {
    contextStore.tabs = tabs
  })
  void window.api.usage.contexts().then((tabs) => {
    if (tabs) contextStore.tabs = tabs
  })
}

// Orange past 40% and red past 50%, well before Claude Code compacts, so the human can wrap up a session first.
export function contextLevel(percent: number): 'normal' | 'warn' | 'danger' {
  if (percent > 50) return 'danger'
  if (percent > 40) return 'warn'
  return 'normal'
}

export function tokenCount(tokens: number): string {
  const short = (n: number, unit: string): string =>
    `${n >= 10 ? Math.round(n) : Math.round(n * 10) / 10}${unit}`
  if (tokens >= 1_000_000) return short(tokens / 1_000_000, 'm')
  if (tokens >= 1000) return short(tokens / 1000, 'k')
  return String(tokens)
}

export function claudeUsage(account: string | undefined): AccountUsage | undefined {
  return usageStore.claude[account ?? 'primary']
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

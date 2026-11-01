// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { Ask, AskState, AskType } from '../../../../preload/types'

const typeLabels: Record<AskType, string> = {
  db: 'Database channel request',
  rester: 'Rester channel request',
  command: 'Command request'
}

const stateLabels: Record<AskState, string> = {
  waiting: 'Waiting for you',
  running: 'Running',
  answered: 'Answered',
  declined: 'Declined',
  expired: 'Expired'
}

export function askTypeLabel(type: AskType): string {
  return typeLabels[type] ?? 'Request'
}

export function askStateLabel(state: AskState): string {
  return stateLabels[state] ?? state
}

// One line for a list row: the first line of the command or message, or the tables asked for.
export function askSummary(ask: Ask): string {
  if (ask.type === 'db') return (ask.tables ?? []).join(', ')
  const text = (ask.type === 'command' ? ask.command : ask.message) ?? ''
  return text.trim().split('\n')[0] ?? ''
}

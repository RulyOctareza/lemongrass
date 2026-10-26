// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

export const GROUP_COLORS = ['coral', 'jade', 'sapphire', 'violet', 'fuchsia', 'teal'] as const

export type GroupColor = (typeof GROUP_COLORS)[number]

export function isGroupColor(value: unknown): value is GroupColor {
  return GROUP_COLORS.includes(value as GroupColor)
}

export function defaultGroupColor(groupId: number): GroupColor {
  return GROUP_COLORS[Math.abs(groupId) % GROUP_COLORS.length]
}

export function groupColorVar(color: GroupColor): string {
  return `var(--color-${color})`
}

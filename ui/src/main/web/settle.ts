// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// A request still in flight after this long is background traffic, such as a notification poll or a long
// poll, and no longer holds an action back.
export const BACKGROUND_AFTER_MS = 2_000

export interface SettleInput {
  loading: boolean
  // How long each open request has been in flight, in milliseconds.
  ages: number[]
  // Milliseconds since the document last changed.
  quiet: number
  quietNeeded: number
}

export interface SettleVerdict {
  settled: boolean
  foreground: number
  background: number
}

export function settleVerdict(input: SettleInput): SettleVerdict {
  const background = input.ages.filter((age) => age >= BACKGROUND_AFTER_MS).length
  const foreground = input.ages.length - background
  return {
    settled: !input.loading && foreground === 0 && input.quiet >= input.quietNeeded,
    foreground,
    background
  }
}

function backgroundNote(count: number): string {
  return count > 0
    ? ` (ignored ${count} background request${count === 1 ? '' : 's'} open for over ${BACKGROUND_AFTER_MS / 1000}s)`
    : ''
}

export function settledText(verdict: SettleVerdict): string {
  return 'settled' + backgroundNote(verdict.background)
}

// Said only when the cap is really hit, naming what was still going on.
export function notSettledText(
  input: SettleInput,
  verdict: SettleVerdict,
  capSeconds: number
): string {
  const reasons: string[] = []
  if (input.loading) reasons.push('the page is still loading')
  if (verdict.foreground > 0) reasons.push(`${verdict.foreground} request(s) still in flight`)
  if (input.quiet < input.quietNeeded)
    reasons.push(`the page changed ${Math.round(input.quiet)} ms ago`)
  return `not settled after ${capSeconds}s: ${reasons.join(', ')}${backgroundNote(verdict.background)}`
}

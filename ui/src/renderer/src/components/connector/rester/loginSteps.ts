// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { VaultLoginStep } from '../../../../../preload/types'

export type StepAction = VaultLoginStep['Action']

// The id keys a row in the list, so a row's inputs stay with its step when steps are removed or moved.
export interface StepDraft extends VaultLoginStep {
  id: number
}

let nextStepId = 0

export function newStep(action: StepAction): StepDraft {
  return { id: nextStepId++, Action: action, URL: '', Selector: '', Field: '', TimeoutSeconds: 0 }
}

export function stepDrafts(login: VaultLoginStep[] | null): StepDraft[] {
  return (login ?? []).map((s) => ({ ...s, id: nextStepId++ }))
}

export function removeStep<T>(steps: T[], index: number): void {
  steps.splice(index, 1)
}

// insertAt is a gap between rows, 0 before the first and steps.length after the last.
export function moveStep<T>(steps: T[], from: number, insertAt: number): void {
  if (from < 0 || from >= steps.length) return
  const target = insertAt > from ? insertAt - 1 : insertAt
  if (target === from) return
  const [step] = steps.splice(from, 1)
  steps.splice(target, 0, step)
}

// Only the fields an action uses are kept, so switching a row's action never saves a stale value.
export function buildStep(s: StepDraft): VaultLoginStep {
  return {
    Action: s.Action,
    URL: s.Action === 'goto' || s.Action === 'wait' ? s.URL.trim() : '',
    Selector: s.Action === 'goto' ? '' : s.Selector.trim(),
    Field: s.Action === 'fill' ? s.Field : '',
    TimeoutSeconds: s.Action === 'wait' ? Math.max(0, Math.round(s.TimeoutSeconds || 0)) : 0
  }
}

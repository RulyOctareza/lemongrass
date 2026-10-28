// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { VaultAppStep } from '../../../../../preload/types'

export type AppStepAction = VaultAppStep['Action']

// The id keys a row in the list, so a row's inputs stay with its step when steps are removed or moved.
export interface AppStepDraft extends VaultAppStep {
  id: number
}

let nextAppStepId = 0

export function newAppStep(action: AppStepAction): AppStepDraft {
  return {
    id: nextAppStepId++,
    Action: action,
    Text: '',
    ID: '',
    Field: '',
    Key: '',
    TimeoutSeconds: 0
  }
}

export function appStepDrafts(login: VaultAppStep[] | null): AppStepDraft[] {
  return (login ?? []).map((s) => ({ ...s, id: nextAppStepId++ }))
}

// Only the fields an action uses are kept, so switching a row's action never saves a stale value.
export function buildAppStep(s: AppStepDraft): VaultAppStep {
  const located = s.Action === 'tap' || s.Action === 'fill' || s.Action === 'wait'
  return {
    Action: s.Action,
    Text: located ? s.Text.trim() : '',
    ID: located ? s.ID.trim() : '',
    Field: s.Action === 'fill' ? s.Field : '',
    Key: s.Action === 'key' ? s.Key.trim() : '',
    TimeoutSeconds:
      s.Action === 'launch' || s.Action === 'key'
        ? 0
        : Math.max(0, Math.round(s.TimeoutSeconds || 0))
  }
}

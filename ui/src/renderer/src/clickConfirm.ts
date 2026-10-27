// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

export type ConfirmState = 'idle' | 'pending' | 'armed'

// Long enough to swallow a habitual double click as well as a mouse that fires twice, so the
// second click of a destructive action is always a deliberate one.
export const CONFIRM_DELAY_MS = 1000

export interface ClickConfirm {
  state(): ConfirmState
  // Returns true when this press is the armed one and the action should run.
  press(): boolean
  reset(): void
}

export function createClickConfirm(
  onChange: (state: ConfirmState) => void,
  delay = CONFIRM_DELAY_MS
): ClickConfirm {
  let state: ConfirmState = 'idle'
  let timer: ReturnType<typeof setTimeout> | undefined

  function set(next: ConfirmState): void {
    if (state === next) return
    state = next
    onChange(next)
  }

  function reset(): void {
    clearTimeout(timer)
    timer = undefined
    set('idle')
  }

  function press(): boolean {
    if (state === 'armed') {
      reset()
      return true
    }
    if (state === 'idle') {
      set('pending')
      timer = setTimeout(() => {
        timer = undefined
        set('armed')
      }, delay)
    }
    return false
  }

  return { state: () => state, press, reset }
}

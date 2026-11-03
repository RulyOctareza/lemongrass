// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'

export type WindowControlsPlacement = 'right' | 'left'

// macOS always shows its native traffic lights, so the placement choice applies elsewhere only.
export const isMac = window.electron.process.platform === 'darwin'

export const windowControls = reactive({ placement: 'right' as WindowControlsPlacement })

let loaded: Promise<void> | undefined

export function loadWindowControls(): Promise<void> {
  loaded ??= window.api.uiState.load().then((state) => {
    windowControls.placement = state.windowControls ?? 'right'
  })
  return loaded
}

export function setWindowControlsPlacement(placement: WindowControlsPlacement): void {
  windowControls.placement = placement
  window.api.uiState.save({ windowControls: placement })
}

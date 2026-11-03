// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { UiState } from '../../preload/types'

export type SidebarView = 'biblio' | 'files'
export type ExplorerView = NonNullable<UiState['explorer']>
export type ExplorerColumn = keyof ExplorerView['columns']

const DEFAULT_VIEW: ExplorerView = {
  columns: { size: false, modified: false, git: true },
  density: 'compact',
  hideIgnored: false
}

export const sidebarSettings = reactive<{ view: SidebarView; explorer: ExplorerView }>({
  view: 'biblio',
  explorer: structuredClone(DEFAULT_VIEW)
})

let loaded = false

export async function loadSidebarSettings(): Promise<void> {
  if (loaded) return
  loaded = true
  const saved = await window.api.uiState.load()
  if (saved.sidebarView) sidebarSettings.view = saved.sidebarView
  if (saved.explorer) {
    sidebarSettings.explorer = {
      ...DEFAULT_VIEW,
      ...saved.explorer,
      columns: { ...DEFAULT_VIEW.columns, ...saved.explorer.columns }
    }
  }
}

export function setSidebarView(view: SidebarView): void {
  sidebarSettings.view = view
  window.api.uiState.save({ sidebarView: view })
}

// ui-state.json merges per top-level field, so the whole explorer object is sent every time.
function saveExplorer(): void {
  window.api.uiState.save({ explorer: JSON.parse(JSON.stringify(sidebarSettings.explorer)) })
}

export function toggleColumn(column: ExplorerColumn): void {
  sidebarSettings.explorer.columns[column] = !sidebarSettings.explorer.columns[column]
  saveExplorer()
}

export function setDensity(density: ExplorerView['density']): void {
  sidebarSettings.explorer.density = density
  saveExplorer()
}

export function toggleHideIgnored(): void {
  sidebarSettings.explorer.hideIgnored = !sidebarSettings.explorer.hideIgnored
  saveExplorer()
}

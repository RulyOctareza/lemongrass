// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ref, type Ref } from 'vue'

export interface ProjectDelete {
  // Shells running for each project; main refuses a delete while a project has any.
  liveShells: Ref<Record<string, number>>
  refreshLiveShells: () => Promise<void>
  refusal: (id: string) => string | undefined
  // True when the project was deleted. A refusal rereads the shells, so the row shows why.
  remove: (id: string) => Promise<boolean>
}

// Shared by the header picker and the start page, so both lists refuse and word a delete the same way.
export function useProjectDelete(): ProjectDelete {
  const liveShells = ref<Record<string, number>>({})

  async function refreshLiveShells(): Promise<void> {
    liveShells.value = await window.api.projects.liveShells()
  }

  function refusal(id: string): string | undefined {
    const n = liveShells.value[id]
    if (!n) return undefined
    return n === 1 ? 'Close its open tab first' : `Close its ${n} open tabs first`
  }

  async function remove(id: string): Promise<boolean> {
    const result = await window.api.projects.delete(id)
    if (result?.ok) return true
    await refreshLiveShells()
    return false
  }

  return { liveShells, refreshLiveShells, refusal, remove }
}

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { AskCount } from '../../preload/types'

// What the renderer knows about asks without the passphrase: the count, and which ask the modal
// shows. The asks themselves are read through the vault with the session passphrase.
export const askState = reactive({
  count: { waiting: 0, blocking: 0, newestId: '' } as AskCount,
  // The ask the modal shows; null while it is closed.
  openId: null as string | null,
  // Bumped on each daemon hint, so open views refetch.
  version: 0
})

export function openAsk(id: string): void {
  askState.openId = id || askState.count.newestId || null
}

export function closeAsk(): void {
  askState.openId = null
}

window.api.ask
  .count()
  .then((count) => (askState.count = count))
  .catch(() => undefined)
window.api.ask.onCount((count) => (askState.count = count))
window.api.ask.onChanged(() => askState.version++)
window.api.ask.onOpen((id) => openAsk(id))

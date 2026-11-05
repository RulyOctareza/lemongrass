<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import IconFrame from './IconFrame.vue'
import { CLAUDE_MARK, CLAUDE_MARK_BOX, CODEX_MARK, CODEX_MARK_BOX } from '../vendorMarks'
type Shape = { d: string; filled?: true }

// The account glyph matches StripIcon's, so Accounts reads as the same thing as the strip's account switch.
const SHAPES = {
  general: [
    { d: 'M2 3.5h2.5M7.5 3.5H12M2 7h5.5M10.5 7H12M2 10.5h1M6 10.5h6' },
    { d: 'M6 2a1.5 1.5 0 1 1 0 3a1.5 1.5 0 1 1 0-3' },
    { d: 'M9 5.5a1.5 1.5 0 1 1 0 3a1.5 1.5 0 1 1 0-3' },
    { d: 'M4.5 9a1.5 1.5 0 1 1 0 3a1.5 1.5 0 1 1 0-3' }
  ],
  appearance: [
    { d: 'M7 2a5 5 0 1 1 0 10a5 5 0 1 1 0-10' },
    { d: 'M7 2a5 5 0 0 0 0 10z', filled: true }
  ],
  accounts: [
    { d: 'M7 1.8a2.2 2.2 0 1 1 0 4.4a2.2 2.2 0 1 1 0-4.4' },
    { d: 'M2.5 12.5c0-2.5 2-4 4.5-4s4.5 1.5 4.5 4' }
  ],
  safety: [{ d: 'M7 1.5l4.5 1.8v3.4c0 2.9-1.9 5-4.5 5.8c-2.6-.8-4.5-2.9-4.5-5.8V3.3z' }],
  updates: [{ d: 'M7 1.8v6.4' }, { d: 'M4.4 5.8L7 8.4l2.6-2.6' }, { d: 'M2 9v2.5h10V9' }],
  dependencies: [
    { d: 'M7 1.6l5 2.6v5.6l-5 2.6l-5-2.6V4.2z' },
    { d: 'M2 4.2l5 2.6l5-2.6' },
    { d: 'M7 6.8v5.6' }
  ],
  credits: [
    {
      d: 'M7 12c-3-2-5.2-4-5.2-6.5a2.6 2.6 0 0 1 5.2-.6a2.6 2.6 0 0 1 5.2 .6c0 2.5-2.2 4.5-5.2 6.5z'
    }
  ]
} as const satisfies Record<string, readonly Shape[]>

export type SettingsIconName = keyof typeof SHAPES | 'claude' | 'codex'

withDefaults(defineProps<{ name: SettingsIconName; size?: number }>(), { size: 13 })
</script>

<template>
  <IconFrame
    v-if="name === 'claude'"
    class="settings-icon"
    :size="size"
    :box="CLAUDE_MARK_BOX"
    :stroke-width="null"
    fill="currentColor"
  >
    <path :d="CLAUDE_MARK" />
  </IconFrame>
  <IconFrame
    v-else-if="name === 'codex'"
    class="settings-icon"
    :size="size"
    :box="CODEX_MARK_BOX"
    :stroke-width="null"
    fill="currentColor"
  >
    <path :d="CODEX_MARK" />
  </IconFrame>
  <IconFrame v-else class="settings-icon" :size="size">
    <path
      v-for="shape in SHAPES[name] as readonly Shape[]"
      :key="shape.d"
      :d="shape.d"
      :fill="shape.filled ? 'currentColor' : undefined"
    />
  </IconFrame>
</template>

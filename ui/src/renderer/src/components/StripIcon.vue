<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The workgroup glyph matches the workgroup tab's icon in tabKinds.ts, so the two read as the same thing.
const SHAPES = {
  account: [
    'M7 1.8a2.2 2.2 0 1 1 0 4.4a2.2 2.2 0 1 1 0-4.4',
    'M2.5 12.5c0-2.5 2-4 4.5-4s4.5 1.5 4.5 4'
  ],
  workgroup: [
    'M7 1.8a1.7 1.7 0 1 1 0 3.4a1.7 1.7 0 1 1 0-3.4',
    'M3 8.8a1.7 1.7 0 1 1 0 3.4a1.7 1.7 0 1 1 0-3.4',
    'M11 8.8a1.7 1.7 0 1 1 0 3.4a1.7 1.7 0 1 1 0-3.4',
    'M6 5l-2 4M8 5l2 4M4.7 10.5h4.6'
  ],
  usage: ['M1.5 10.5a5.5 5.5 0 0 1 11 0', 'M7 10.5l2.6-3.2'],
  context: ['M2.5 2.5h9v9h-9z', 'M4.5 5h5', 'M4.5 7h5', 'M4.5 9h2.5']
} as const

export type StripIconName = keyof typeof SHAPES

defineProps<{
  name: StripIconName
}>()
</script>

<template>
  <svg
    class="strip-icon"
    width="13"
    height="13"
    viewBox="0 0 14 14"
    fill="none"
    stroke="currentColor"
    stroke-width="1.2"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path v-for="d in SHAPES[name]" :key="d" :d="d" />
  </svg>
</template>

<style scoped>
.strip-icon {
  flex-shrink: 0;
  color: var(--color-fg-muted);
}
</style>

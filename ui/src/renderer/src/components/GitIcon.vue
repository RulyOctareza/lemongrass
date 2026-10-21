<script setup lang="ts">
const SHAPES = {
  branch: [
    'M4 1.5a1.5 1.5 0 1 1 0 3a1.5 1.5 0 1 1 0-3',
    'M4 9.5a1.5 1.5 0 1 1 0 3a1.5 1.5 0 1 1 0-3',
    'M10.5 4a1.5 1.5 0 1 1 0 3a1.5 1.5 0 1 1 0-3',
    'M4 4.5v5M10.5 7c0 2.2-2.2 2.9-6 3.2'
  ],
  switch: ['M2 4.5h9.5M9 2l2.5 2.5L9 7', 'M12 9.5H2.5M5 7L2.5 9.5 5 12'],
  open: [
    'M8 2h4v4',
    'M12 2L6.5 7.5',
    'M10.5 8v3.5a1 1 0 0 1-1 1H2.5a1 1 0 0 1-1-1V4.5a1 1 0 0 1 1-1H6'
  ],
  fetch: ['M7 2v7M4 6.5l3 3 3-3', 'M2.5 12h9'],
  push: ['M7 12V5M4 7.5l3-3 3 3', 'M2.5 2h9'],
  spinner: ['M7 2a5 5 0 1 1-5 5'],
  change: ['M9.5 2.5l2 2L5 11l-2.5.5L3 9z'],
  stash: ['M2 3.5h10V6H2z', 'M3 6v5.5h8V6', 'M5.5 8.5h3'],
  apply: ['M4.5 4.5h7v7h-7z', 'M9.5 4.5v-2h-7v7h2'],
  pop: ['M7 9V2M4 5l3-3 3 3', 'M2.5 12h9'],
  drop: ['M2.5 4h9', 'M5.5 4V2.5h3V4', 'M3.5 4l.5 7.5h6L10.5 4'],
  pushed: ['M3 7.5l3 3 5-6'],
  unpushed: ['M7 11.5V3M4 6l3-3 3 3']
} as const

export type GitIconName = keyof typeof SHAPES

defineProps<{
  name: GitIconName
}>()
</script>

<template>
  <svg
    class="git-icon"
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
.git-icon {
  flex-shrink: 0;
}
</style>

<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import type { MatchRange } from '../../treeSearch'

const props = defineProps<{
  text: string
  ranges?: MatchRange[]
}>()

// Ranges past the end of the shown text (a hidden .md suffix) are clipped away.
const parts = computed(() => {
  const out: { text: string; hit: boolean }[] = []
  let at = 0
  for (const [start, end] of props.ranges ?? []) {
    const from = Math.min(start, props.text.length)
    const to = Math.min(end, props.text.length)
    if (from >= to) continue
    if (from > at) out.push({ text: props.text.slice(at, from), hit: false })
    out.push({ text: props.text.slice(from, to), hit: true })
    at = to
  }
  if (at < props.text.length) out.push({ text: props.text.slice(at), hit: false })
  return out
})
</script>

<template>
  <span class="match-name"
    ><template v-for="(part, index) in parts" :key="index"
      ><mark v-if="part.hit" class="hit">{{ part.text }}</mark
      ><template v-else>{{ part.text }}</template></template
    ></span
  >
</template>

<style scoped>
.match-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hit {
  background: transparent;
  color: var(--color-fg-accent);
  font-weight: var(--weight-semibold);
}
</style>

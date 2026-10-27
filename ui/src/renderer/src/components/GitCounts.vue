<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed } from 'vue'

// A branch against its upstream: ahead is what the branch has that the origin lacks, behind the
// other way round. A zero side is not shown.
const props = defineProps<{
  ahead: number
  behind: number
}>()

const title = computed(() => {
  const parts: string[] = []
  if (props.ahead > 0) parts.push(`${props.ahead} ahead of the origin`)
  if (props.behind > 0) parts.push(`${props.behind} behind the origin`)
  return parts.join(', ')
})
</script>

<template>
  <span v-if="ahead > 0 || behind > 0" class="git-counts" :title="title">
    <span v-if="ahead > 0" class="git-count ahead">
      <svg
        width="9"
        height="9"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.3"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M5 8.5v-7M2 4.5l3-3 3 3" />
      </svg>
      {{ ahead }}
    </span>
    <span v-if="behind > 0" class="git-count behind">
      <svg
        width="9"
        height="9"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.3"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M5 1.5v7M2 5.5l3 3 3-3" />
      </svg>
      {{ behind }}
    </span>
  </span>
</template>

<style scoped>
.git-counts {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: var(--space-2);
  white-space: nowrap;
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
}

.git-count {
  display: flex;
  align-items: center;
  gap: 2px;
}

/* Ahead matches the Push button's green, behind the Fetch button's blue. */
.git-count.ahead {
  color: var(--color-success);
}

.git-count.behind {
  color: var(--color-info);
}
</style>

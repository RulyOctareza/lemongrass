<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import type { GitDiff, GitDiffLine } from '../../../preload/types'

defineProps<{
  diff: GitDiff | null
}>()

function cellClass(line: GitDiffLine | null): string {
  if (!line) return 'blank'
  return line.type
}
</script>

<template>
  <p v-if="!diff" class="notice">Loading...</p>
  <p v-else-if="diff.status === 'binary'" class="notice">Binary file, no preview.</p>
  <p v-else-if="diff.status === 'too-large'" class="notice">This diff is too large to display.</p>
  <p v-else-if="diff.status === 'empty'" class="notice">
    No textual changes. The file may have a mode change or be empty.
  </p>
  <p v-else-if="diff.status === 'error'" class="notice error">{{ diff.message }}</p>
  <div v-else class="diff-scroll">
    <template v-for="(hunk, hunkIndex) in diff.hunks" :key="hunkIndex">
      <div class="hunk-header">{{ hunk.header }}</div>
      <div v-for="(row, rowIndex) in hunk.rows" :key="rowIndex" class="diff-row">
        <span class="line-number" :class="cellClass(row.left)">{{ row.left?.number }}</span>
        <span class="line-text" :class="cellClass(row.left)">{{ row.left?.text }}</span>
        <span class="line-number" :class="cellClass(row.right)">{{ row.right?.number }}</span>
        <span class="line-text" :class="cellClass(row.right)">{{ row.right?.text }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.notice {
  padding: var(--space-4) var(--space-6);
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}

.notice.error {
  color: var(--color-error);
  white-space: pre-wrap;
}

.diff-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  line-height: 1.5;
}

.hunk-header {
  padding: 2px var(--space-4);
  background: var(--color-surface-2);
  color: var(--color-fg-secondary);
  white-space: pre;
  overflow: hidden;
  text-overflow: ellipsis;
}

.diff-row {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr) 44px minmax(0, 1fr);
}

.line-number {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
  text-align: right;
  user-select: none;
}

.line-text {
  padding: 0 var(--space-2);
  color: var(--color-fg-primary);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.line-text:nth-child(2) {
  border-right: 1px solid var(--color-border-subtle);
}

.del {
  background: var(--color-error-muted);
}

.add {
  background: var(--color-success-muted);
}

.blank {
  background: var(--color-surface-1);
}
</style>

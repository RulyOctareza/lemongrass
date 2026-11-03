<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import AppEmptyState from './AppEmptyState.vue'
import type { DocEntry } from '../workspace'

// A file shown read-only by its kind: text with a line-number gutter, an image, or a one-line
// message for binary and too-large files. A biblio doc and a project file tab both render this.
const props = defineProps<{
  entry: DocEntry
  imageSrc: string
  alt: string
}>()

const lineNumbers = computed(() => {
  const count = props.entry.kind === 'text' ? props.entry.content.split('\n').length : 0
  return Array.from({ length: count }, (_, i) => i + 1).join('\n')
})
</script>

<template>
  <div v-if="entry.kind === 'text'" class="file-text">
    <pre class="gutter" aria-hidden="true">{{ lineNumbers }}</pre>
    <pre class="code">{{ entry.content }}</pre>
  </div>
  <div v-else-if="entry.kind === 'image'" class="file-image">
    <img :src="imageSrc" :alt="alt" />
  </div>
  <AppEmptyState
    v-else-if="entry.kind === 'binary'"
    layout="fill"
    title="This file is binary and has no preview."
  />
  <AppEmptyState
    v-else-if="entry.kind === 'tooLarge'"
    layout="fill"
    title="This file is over 2 MB and is not shown here."
  />
</template>

<style scoped>
.file-text {
  flex: 1;
  min-height: 0;
  overflow: auto;
  display: flex;
  font-family: var(--font-mono);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.file-text pre {
  margin: 0;
  padding: var(--space-4) var(--space-3);
  font: inherit;
  white-space: pre;
  tab-size: 4;
}

.gutter {
  position: sticky;
  left: 0;
  flex: none;
  text-align: right;
  user-select: none;
  color: var(--color-fg-muted);
  background: var(--color-surface-1);
  border-right: 1px solid var(--color-border-subtle);
}

.code {
  flex: 1;
  color: var(--color-fg-primary);
}

.file-image {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-4);
}

.file-image img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}
</style>

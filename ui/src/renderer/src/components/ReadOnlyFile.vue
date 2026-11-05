<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import AppEmptyState from './AppEmptyState.vue'
import CodeReader from './CodeReader.vue'
import type { DocEntry } from '../workspace'

// A file shown read-only by its kind: syntax-colored text with a line-number gutter, an image, or
// a one-line message for binary and too-large files. A biblio doc and a project file tab both
// render this; `path` picks the grammar.
defineProps<{
  entry: DocEntry
  path: string
  imageSrc: string
  alt: string
}>()
</script>

<template>
  <CodeReader v-if="entry.kind === 'text'" :content="entry.content" :path="path" />
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

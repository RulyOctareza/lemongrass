<script setup lang="ts">
import { ref, watch } from 'vue'
import { formatAge, formatDate } from '../gitTime'
import type { ProjectRef } from '../workspace'
import type { GitBlame } from '../../../preload/types'

const props = defineProps<{
  project: ProjectRef
  hash: string
  path: string
}>()

const blame = ref<GitBlame | null>(null)
let requestId = 0

async function load(): Promise<void> {
  const id = ++requestId
  blame.value = null
  const next = await window.api.git.blame(props.project.path, props.hash, props.path)
  if (id === requestId) blame.value = next
}

watch(() => [props.hash, props.path], load, { immediate: true })

// Meta shows on the first line of each run of lines from one commit.
function startsRun(index: number): boolean {
  if (blame.value?.status !== 'ok') return false
  return index === 0 || blame.value.lines[index - 1].hash !== blame.value.lines[index].hash
}
</script>

<template>
  <p v-if="!blame" class="notice">Loading...</p>
  <p v-else-if="blame.status === 'too-large'" class="notice">This file is too large to blame.</p>
  <p v-else-if="blame.status === 'binary'" class="notice">Binary file, no blame.</p>
  <p v-else-if="blame.status === 'error'" class="notice error">{{ blame.message }}</p>
  <div v-else class="blame-scroll">
    <div
      v-for="(line, index) in blame.lines"
      :key="line.number"
      class="blame-row"
      :class="{ run: startsRun(index) }"
    >
      <span class="blame-hash" :title="`${line.hash}\n${formatDate(line.time)}`">{{
        startsRun(index) ? line.hash.slice(0, 7) : ''
      }}</span>
      <span class="blame-author">{{ startsRun(index) ? line.author : '' }}</span>
      <span class="blame-age">{{ startsRun(index) ? formatAge(line.time) : '' }}</span>
      <span class="blame-number">{{ line.number }}</span>
      <span class="blame-text">{{ line.text }}</span>
    </div>
  </div>
</template>

<style scoped>
.blame-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  line-height: 1.5;
}

.blame-row {
  display: grid;
  grid-template-columns: 64px 110px 36px 44px minmax(0, 1fr);
}

.blame-row.run {
  border-top: 1px solid var(--color-border-subtle);
}

.blame-hash {
  padding: 0 var(--space-2);
  color: var(--color-fg-accent);
}

.blame-author {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-secondary);
}

.blame-age {
  color: var(--color-fg-muted);
  text-align: right;
}

.blame-number {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
  text-align: right;
  user-select: none;
}

.blame-text {
  padding: 0 var(--space-2);
  color: var(--color-fg-primary);
  white-space: pre;
}

.notice {
  padding: var(--space-4) var(--space-6);
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}

.notice.error {
  color: var(--color-error);
  white-space: pre-wrap;
}
</style>

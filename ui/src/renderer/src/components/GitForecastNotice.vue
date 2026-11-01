<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import DepRow from './deps/DepRow.vue'
import { depsStore } from '../deps'
import type { GitForecast } from '../../../preload/types'

// null while the forecast is still running.
const props = defineProps<{
  forecast: GitForecast | null
  kind: 'merge' | 'rebase'
}>()

// Read from the live dependency state, so the row follows an install started from it.
const needed = computed(() => {
  const forecast = props.forecast
  if (forecast?.status !== 'needsDependency') return null
  return depsStore.state?.report?.deps.find((dep) => dep.name === forecast.dep) ?? null
})

const neededText = computed(() => {
  const forecast = props.forecast
  if (forecast?.status !== 'needsDependency') return ''
  const floor = needed.value?.floor
  const want = floor ? `${forecast.dep} ${floor} or newer` : forecast.dep
  return `Conflicts cannot be checked ahead of time. This check needs ${want}.`
})

const conflict = computed(() => (props.forecast?.status === 'conflict' ? props.forecast : null))

// Split like the git panel's conflicted file rows, so a file reads the same here and there.
function fileName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

function fileDir(path: string): string {
  const index = path.lastIndexOf('/')
  return index === -1 ? '' : path.slice(0, index)
}

function files(count: number): string {
  return count === 1 ? '1 file' : `${count} files`
}

const heading = computed(() => {
  const found = conflict.value
  if (!found) return ''
  if (found.stop) {
    return `This rebase will stop with a conflict at commit ${found.stop.index} of ${found.stop.total}, ${found.stop.subject}, in:`
  }
  return `This ${props.kind} will conflict in ${files(found.paths.length)}:`
})
</script>

<template>
  <p v-if="!forecast" class="forecast-note">Checking for conflicts...</p>
  <p v-else-if="forecast.status === 'clean'" class="forecast-note">No conflicts expected.</p>
  <p v-else-if="forecast.status === 'unavailable'" class="forecast-note" :title="forecast.error">
    Could not check for conflicts.
  </p>
  <div v-else-if="forecast.status === 'needsDependency'" class="forecast-dependency">
    <p class="forecast-note">{{ neededText }}</p>
    <ul v-if="needed && depsStore.state" class="settings-rows">
      <DepRow :dep="needed" :state="depsStore.state" />
    </ul>
  </div>
  <div v-else-if="conflict" class="forecast-conflict">
    <p>{{ heading }}</p>
    <ul class="forecast-paths">
      <li v-for="path in conflict.paths" :key="path" :title="path">
        <span class="file-name">{{ fileName(path) }}</span>
        <span class="file-dir">{{ fileDir(path) }}</span>
      </li>
    </ul>
    <p v-if="conflict.stop && conflict.stop.index < conflict.stop.total" class="forecast-later">
      Later commits are checked once this one is resolved.
    </p>
  </div>
</template>

<style scoped>
.forecast-dependency {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.forecast-note {
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}

.forecast-conflict {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-left: 3px solid var(--color-error);
  border-radius: var(--radius-md);
  background: var(--color-surface-1);
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  word-break: break-word;
}

.forecast-paths {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
  max-height: 10rem;
  overflow-y: auto;
}

.forecast-paths li {
  display: flex;
  gap: 6px;
  min-width: 0;
}

.file-name {
  flex-shrink: 0;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-warning);
}

.file-dir {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-muted);
}

.forecast-later {
  color: var(--color-fg-secondary);
}
</style>

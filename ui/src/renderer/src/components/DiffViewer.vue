<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { gitStateOf } from '../git'
import type { ProjectRef } from '../workspace'
import type { GitDiff } from '../../../preload/types'
import DiffBody from './DiffBody.vue'

const props = defineProps<{
  project: ProjectRef
  path: string
}>()

const state = computed(() => gitStateOf(props.project.id))
const file = computed(() => state.value.status?.files.find((f) => f.path === props.path))
const diff = ref<GitDiff | null>(null)
let requestId = 0

async function load(): Promise<void> {
  const current = file.value
  if (!current) {
    diff.value = null
    return
  }
  const id = ++requestId
  const next = await window.api.git.diff(props.project.path, current.path, current.origPath)
  if (id !== requestId) return
  if (JSON.stringify(next) !== JSON.stringify(diff.value)) diff.value = next
}

watch(
  () => props.path,
  () => {
    diff.value = null
    void load()
  }
)

watch(
  () => [state.value.revision, file.value?.kind, file.value?.origPath],
  () => void load(),
  { immediate: true }
)
</script>

<template>
  <div class="diff-viewer">
    <div class="diff-header">
      <span v-if="file" class="diff-kind" :class="file.group">{{ file.kind }}</span>
      <span class="diff-path" :title="path">{{ path }}</span>
      <span v-if="file?.origPath" class="diff-orig">from {{ file.origPath }}</span>
      <span v-if="diff?.status === 'ok'" class="diff-stats">
        <span class="stat-add">+{{ diff.added }}</span>
        <span class="stat-del">-{{ diff.removed }}</span>
      </span>
    </div>

    <p v-if="!state.status" class="notice">Loading...</p>
    <p v-else-if="!file" class="notice">No changes in this file.</p>
    <DiffBody v-else :diff="diff" />
  </div>
</template>

<style scoped>
.diff-viewer {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.diff-header {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-4);
  border-bottom: 1px solid var(--color-border-subtle);
  font-size: var(--text-xs);
}

.diff-kind {
  flex-shrink: 0;
  padding: 0 var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-surface-2);
  color: var(--color-fg-secondary);
  text-transform: capitalize;
}

.diff-kind.stagedNew {
  color: var(--color-success);
}

.diff-kind.unstagedNew {
  color: var(--color-error);
}

.diff-kind.deleted {
  color: var(--color-deleted);
}

.diff-kind.conflicted {
  color: var(--color-warning);
}

.diff-kind.changed,
.diff-kind.renamed {
  color: var(--color-info);
}

.diff-path {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-primary);
  font-family: var(--font-mono);
}

.diff-orig {
  flex-shrink: 0;
  color: var(--color-fg-muted);
}

.diff-stats {
  flex-shrink: 0;
  margin-left: auto;
  display: flex;
  gap: var(--space-2);
  font-family: var(--font-mono);
}

.stat-add {
  color: var(--color-success);
}

.stat-del {
  color: var(--color-error);
}

.notice {
  padding: var(--space-4) var(--space-6);
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}
</style>

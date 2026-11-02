<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import AppConfirmButton from '../AppConfirmButton.vue'
import { statusClass } from '../../resterClientCall'
import { formatAge } from '../../relativeAge'
import { ageNow } from '../../ageClock'
import type { ResterClientRun } from '../../../../preload/types'
import AppEmptyState from '../AppEmptyState.vue'

defineProps<{
  runs: ResterClientRun[]
  selectedRunId: string | null
}>()

const emit = defineEmits<{
  pick: [run: ResterClientRun]
  remove: [runId: string]
}>()

function statusOf(run: ResterClientRun): { text: string; className: string } {
  if (run.error) return { text: 'Error', className: 'failed' }
  const status = run.result?.status ?? (run.download?.kind === 'saved' ? run.download.status : 0)
  return { text: String(status), className: statusClass(status) }
}
</script>

<template>
  <div class="run-list">
    <AppEmptyState
      v-if="runs.length === 0"
      size="sm"
      title="No runs yet."
      detail="Each send is kept here."
    />
    <div
      v-for="run in runs"
      :key="run.id"
      class="run"
      :class="{ active: run.id === selectedRunId }"
      :title="new Date(run.at).toLocaleString()"
      @click="emit('pick', run)"
    >
      <div class="run-line">
        <span class="method">{{ run.request.method }}</span>
        <span class="status" :class="statusOf(run).className">{{ statusOf(run).text }}</span>
        <span v-if="run.download?.kind === 'saved'" class="tag">Download</span>
        <span class="when">{{ formatAge(run.at, ageNow) }}</span>
        <AppConfirmButton
          class="remove"
          title="Delete run"
          @click.stop
          @confirm="emit('remove', run.id)"
        >
          ×
        </AppConfirmButton>
      </div>
      <div class="run-line detail">
        <span class="user">{{ run.request.user }}</span>
        <span class="path">{{ run.request.url }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.run-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.run {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2);
  border-radius: var(--radius-md);
  cursor: pointer;
}

.run:hover {
  background: var(--color-surface-2);
}

.run.active {
  background: var(--color-surface-3);
}

.run-line {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}

.method {
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: var(--weight-bold);
  color: var(--color-fg-secondary);
}

.tag {
  color: var(--color-fg-muted);
  font-size: 10px;
}

.when {
  margin-left: auto;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  white-space: nowrap;
}

.detail {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.user {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
}

.path {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
}

.remove {
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  cursor: pointer;
  visibility: hidden;
}

.run:hover .remove {
  visibility: visible;
}

.remove:hover {
  background: var(--color-surface-3);
  color: var(--color-fg-primary);
}
</style>

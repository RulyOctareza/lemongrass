<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { gitStateOf } from '../git'
import type { ProjectRef } from '../workspace'
import type { GitDiff, GitDiffLine } from '../../../preload/types'

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

function cellClass(line: GitDiffLine | null): string {
  if (!line) return 'blank'
  return line.type
}
</script>

<template>
  <div class="diff-viewer">
    <div class="diff-header">
      <span v-if="file" class="diff-kind" :class="file.kind">{{ file.kind }}</span>
      <span class="diff-path" :title="path">{{ path }}</span>
      <span v-if="file?.origPath" class="diff-orig">from {{ file.origPath }}</span>
      <span v-if="diff?.status === 'ok'" class="diff-stats">
        <span class="stat-add">+{{ diff.added }}</span>
        <span class="stat-del">-{{ diff.removed }}</span>
      </span>
    </div>

    <p v-if="!state.status" class="notice">Loading...</p>
    <p v-else-if="!file" class="notice">No changes in this file.</p>
    <p v-else-if="!diff" class="notice">Loading...</p>
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

.diff-kind.added,
.diff-kind.untracked {
  color: var(--color-success);
}

.diff-kind.deleted,
.diff-kind.conflicted {
  color: var(--color-error);
}

.diff-kind.modified,
.diff-kind.typechange {
  color: var(--color-warning);
}

.diff-kind.renamed,
.diff-kind.copied {
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

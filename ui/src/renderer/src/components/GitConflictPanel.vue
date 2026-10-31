<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script lang="ts">
import { reactive } from 'vue'

// Project ids whose Conflicted files drawer is open. It starts folded and stays as the human left
// it while the panel folds and unfolds, which remounts this component.
const openDrawers = reactive(new Set<string>())
</script>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { finishOperation, gitStateOf } from '../git'
import { openConflict, type ProjectRef } from '../workspace'
import { handConflictTo, useConflictThread } from '../conflictThread'
import { fileStatus, fileStatusText, type ConflictFileStatus } from '../conflictThreadModel'
import { shellTitles } from '../shellRegistry'
import type { GitFile } from '../../../preload/types'
import AppButton from './AppButton.vue'
import AppSplitButton from './AppSplitButton.vue'
import ResolveWithMenu from './ResolveWithMenu.vue'

const props = defineProps<{
  project: ProjectRef
}>()

const state = computed(() => gitStateOf(props.project.id))
const status = computed(() => state.value.status)
const error = ref<string | null>(null)
const conflicted = computed(() =>
  (status.value?.files ?? []).filter((file) => file.group === 'conflicted')
)
const drawerOpen = computed(() => openDrawers.has(props.project.id))
const thread = useConflictThread(() => props.project)
const handing = ref(false)
const menu = ref<InstanceType<typeof ResolveWithMenu> | null>(null)
const split = ref<InstanceType<typeof AppSplitButton> | null>(null)
const manualItems = [{ key: 'manual', label: 'Resolve manually' }]

// The tab list hangs off the whole split button, not only its main part.
function openTabs(): void {
  menu.value?.toggle(split.value?.el ?? null)
}

function onManual(key: string): void {
  if (key === 'manual') resolve()
}

// The tab the conflict is handed to, by its live title, while its thread is open.
const assignee = computed(() => {
  const tabId = thread.value.status?.assignee
  return tabId ? (shellTitles[tabId] ?? 'a tab that is closed') : null
})

function statusOf(file: GitFile): ConflictFileStatus | null {
  return fileStatus(thread.value.messages, file.path, file.conflictBlocks)
}

async function handOver(tabId: string): Promise<void> {
  handing.value = true
  error.value = null
  const refused = await handConflictTo(props.project, tabId)
  handing.value = false
  if (refused) error.value = refused
  else menu.value?.hide()
}

// The refs are split out so the sentence can set them apart; without refs it is a plain sentence.
const message = computed(() => {
  const refs = status.value?.operationRefs ?? null
  if (status.value?.operation === 'merge') {
    return refs
      ? { lead: 'Conflict while trying to merge', from: refs.from, join: 'into', onto: refs.onto }
      : { lead: 'Conflict while merging.' }
  }
  return refs
    ? { lead: 'Conflict while rebasing', from: refs.from, join: 'onto', onto: refs.onto }
    : { lead: 'Conflict while rebasing.' }
})

function fileName(file: GitFile): string {
  return file.path.slice(file.path.lastIndexOf('/') + 1)
}

function fileDir(file: GitFile): string {
  const index = file.path.lastIndexOf('/')
  return index === -1 ? '' : file.path.slice(0, index)
}

function blocksTitle(file: GitFile): string {
  if (file.conflictBlocks == null) return file.path
  const blocks =
    file.conflictBlocks === 1 ? '1 conflict block' : `${file.conflictBlocks} conflict blocks`
  return `${file.path}: ${blocks} left`
}

function toggleDrawer(): void {
  if (openDrawers.has(props.project.id)) openDrawers.delete(props.project.id)
  else openDrawers.add(props.project.id)
}

function resolve(): void {
  if (conflicted.value.length > 0) openConflict(props.project, conflicted.value[0].path)
}

async function finish(action: 'continue' | 'abort'): Promise<void> {
  error.value = await finishOperation(props.project, action)
}
</script>

<template>
  <div class="conflict-panel">
    <p class="conflict-message">
      {{ message.lead }}
      <template v-if="message.from">
        <span class="conflict-ref">{{ message.from }}</span>
        {{ message.join }}
        <span class="conflict-ref">{{ message.onto }}</span>
      </template>
    </p>
    <p v-if="error" class="conflict-error">{{ error }}</p>
    <div class="conflict-buttons">
      <template v-if="conflicted.length > 0">
        <AppSplitButton
          ref="split"
          variant="primary"
          class="conflict-split"
          :disabled="state.busy !== null"
          :items="manualItems"
          menu-label="Other ways to resolve"
          title="Hand the whole conflict to a Claude or Codex tab"
          @click="openTabs"
          @menu="menu?.hide()"
          @select="onManual"
        >
          Resolve with model
        </AppSplitButton>
        <ResolveWithMenu ref="menu" :project="project" :sending="handing" @pick="handOver" />
      </template>
      <AppButton
        v-else
        variant="primary"
        class="conflict-action"
        :disabled="state.busy !== null"
        @click="finish('continue')"
      >
        Continue
      </AppButton>
      <AppButton
        variant="danger"
        class="conflict-abort"
        :disabled="state.busy !== null"
        @click="finish('abort')"
      >
        Abort
      </AppButton>
    </div>
    <p v-if="assignee" class="conflict-assignee">
      Handed to <span class="assignee-name">{{ assignee }}</span>
    </p>

    <p v-if="conflicted.length === 0" class="conflict-empty">All conflicts are resolved.</p>
    <section v-else>
      <button class="drawer-head" :aria-expanded="drawerOpen" @click="toggleDrawer">
        <svg
          class="drawer-chevron"
          :class="{ open: drawerOpen }"
          width="9"
          height="9"
          viewBox="0 0 10 10"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M3 2l3 3-3 3" />
        </svg>
        <span>Conflicted files</span>
        <span class="drawer-count">{{ conflicted.length }}</span>
      </button>
      <ul v-if="drawerOpen" class="conflict-files">
        <li v-for="file in conflicted" :key="file.path">
          <button
            class="conflict-file"
            :title="blocksTitle(file)"
            @click="openConflict(project, file.path)"
          >
            <span class="file-name">{{ fileName(file) }}</span>
            <span class="file-dir">{{ fileDir(file) }}</span>
            <span v-if="statusOf(file)" class="file-status" :class="statusOf(file)!.kind">{{
              fileStatusText(statusOf(file)!)
            }}</span>
            <span v-if="file.conflictBlocks != null" class="file-blocks">{{
              file.conflictBlocks
            }}</span>
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.conflict-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-size: var(--text-xs);
}

.conflict-message {
  padding: 0 var(--space-2);
  color: var(--color-fg-primary);
}

.conflict-ref {
  color: var(--color-fg-accent);
  font-family: var(--font-mono);
  font-weight: var(--weight-medium);
  word-break: break-all;
}

.conflict-error {
  max-height: 120px;
  overflow-y: auto;
  padding: 0 var(--space-2);
  color: var(--color-error);
  white-space: pre-wrap;
  word-break: break-word;
}

.conflict-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  padding: 0 var(--space-2);
}

/* On one line the split button takes nearly all the free width, its grow factor dwarfing Abort's.
   At the column's narrowest width the two do not fit, so Abort wraps below and, alone on its line,
   grows to the full width as the split button above it. */
.conflict-buttons .conflict-split {
  flex: 1000 1 160px;
}

.conflict-buttons .conflict-action {
  flex: 1;
  padding: var(--space-1) var(--space-2);
}

.conflict-buttons .conflict-abort {
  flex: 1 0 auto;
  padding: var(--space-1) var(--space-3);
}

.conflict-assignee {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
}

.assignee-name {
  color: var(--color-fg-primary);
}

.conflict-empty {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
}

.drawer-head {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 3px var(--space-2);
  background: transparent;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  cursor: pointer;
}

.drawer-head:hover {
  color: var(--color-fg-primary);
}

.drawer-chevron {
  flex-shrink: 0;
  transition: transform var(--duration-fast) var(--ease-out);
}

.drawer-chevron.open {
  transform: rotate(90deg);
}

.drawer-count {
  color: var(--color-fg-muted);
  font-weight: var(--weight-regular);
}

.conflict-files {
  max-height: 240px;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 0;
}

.conflict-file {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 3px var(--space-2) 3px var(--space-5);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-warning);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.conflict-file:hover {
  background: var(--color-surface-2);
}

.file-name {
  flex-shrink: 1;
  min-width: 0;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-dir {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-muted);
}

.file-status {
  flex-shrink: 0;
  padding: 0 var(--space-1);
  border-radius: var(--radius-sm);
  white-space: nowrap;
}

/* Violet is the model's work, as in the resolver. */
.file-status.proposed {
  background: var(--color-violet-muted);
  color: var(--color-violet);
}

.file-status.markers {
  background: var(--color-error-muted);
  color: var(--color-error);
}

.file-blocks {
  flex-shrink: 0;
  color: var(--color-fg-muted);
  font-variant-numeric: tabular-nums;
}
</style>

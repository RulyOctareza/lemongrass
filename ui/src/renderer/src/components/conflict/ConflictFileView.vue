<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onActivated, onDeactivated, shallowRef, watch } from 'vue'
import { gitStateOf } from '../../git'
import { dropDraft } from '../../conflictDrafts'
import { conflictScope } from '../../conflictAnnotations'
import type { ProjectRef } from '../../workspace'
import type { GitConflictFile } from '../../../../preload/types'
import ConflictMerge from './ConflictMerge.vue'
import ConflictSides from './ConflictSides.vue'
import ConflictThreadBar from './ConflictThreadBar.vue'
import type { ConflictThreadState } from '../../conflictThread'
import { latestSubmit, notesFor } from '../../conflictThreadModel'

// One file of the conflict tab, cached by the tab while another file is shown. The picker slot is
// the tab's file picker, placed at the start of whichever top row this file shows.
const props = defineProps<{
  project: ProjectRef
  path: string
  thread: ConflictThreadState
}>()

const file = shallowRef<GitConflictFile | null>(null)
const okFile = computed(() => (file.value?.status === 'ok' ? file.value : null))
let loading = false
let active = false

// The model's latest submission, when it lists this file, puts the middle in proposed mode.
const submitId = computed(() => {
  const submit = latestSubmit(props.thread.messages)
  return submit && submit.files.includes(props.path) ? submit.id : null
})
const notes = computed(() => notesFor(props.thread.messages, props.path))

// Reloaded on every status refresh while shown, so a file the model changed or resolved shows here,
// and once each time it is shown again. The object is replaced only when it differs, since the
// merge screen reacts to every new one.
async function load(): Promise<void> {
  if (loading) return
  loading = true
  try {
    const next = await window.api.git.conflict(props.project.path, props.path)
    if (JSON.stringify(next) !== JSON.stringify(file.value)) file.value = next
    // A resolved file has no work left to keep for its tab.
    if (next.status === 'notConflicted') dropDraft(props.project.id, props.path)
  } catch (err) {
    file.value = { status: 'error', error: err instanceof Error ? err.message : String(err) }
  } finally {
    loading = false
  }
}

// The tab always caches this view, so activation also covers the first mount.
onActivated(() => {
  active = true
  void load()
})
onDeactivated(() => {
  active = false
})
watch(
  () => gitStateOf(props.project.id).revision,
  () => {
    if (active) void load()
  }
)
</script>

<template>
  <div class="conflict-view">
    <!-- The merge screen puts the picker in its own toolbar; every other state gets this row. -->
    <div v-if="!okFile?.chunks" class="file-bar">
      <slot name="picker" />
    </div>
    <div v-if="!file" class="notice">Loading {{ path }}</div>
    <div v-else-if="file.status === 'notConflicted'" class="notice done">
      {{ path }} is not in conflict. Pick another file above, or close this tab.
    </div>
    <div v-else-if="file.status === 'error'" class="notice error">{{ file.error }}</div>
    <template v-else-if="okFile">
      <ConflictMerge
        v-if="okFile.chunks"
        :project="project"
        :path="path"
        :file="okFile"
        :submit-id="submitId"
        :notes="notes"
      >
        <template #picker><slot name="picker" /></template>
      </ConflictMerge>
      <ConflictSides v-else :project="project" :path="path" :file="okFile" />
      <!-- The conflict thread takes the review bar's place in the conflict scope: line comments go
           out as anchored messages on Send instead of one pasted review. -->
      <ConflictThreadBar
        :project="project"
        :scope="conflictScope(okFile.operation, okFile.leftRef, okFile.rightRef)"
        :thread="thread"
      />
    </template>
  </div>
</template>

<style scoped>
.conflict-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--color-surface-0);
}

.file-bar {
  display: flex;
  align-items: center;
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border-subtle);
}

.notice {
  padding: var(--space-6);
  font-size: var(--text-sm);
  color: var(--color-fg-secondary);
}

/* The last thing a resolved file shows, so it sits in the middle of the tab where it is seen. */
.notice.done {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  color: var(--color-fg-primary);
}

.notice.error {
  color: var(--color-error);
}
</style>

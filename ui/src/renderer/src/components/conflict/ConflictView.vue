<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, watch } from 'vue'
import { gitStateOf } from '../../git'
import { useConflictThread } from '../../conflictThread'
import { setConflictPath, type ProjectRef } from '../../workspace'
import ConflictFilePicker from './ConflictFilePicker.vue'
import ConflictFileView from './ConflictFileView.vue'

// The conflict tab: one for the whole conflict session, showing one file at a time.
const props = defineProps<{
  project: ProjectRef
  path: string
  tabId: string
}>()

// Read once here for every file view and the picker, since each reader refreshes the thread on
// every git status change.
const thread = useConflictThread(() => props.project)

const status = computed(() => gitStateOf(props.project.id).status)
const conflicted = computed(() =>
  (status.value?.files ?? []).filter((file) => file.group === 'conflicted').map((file) => file.path)
)
// Shown in place of a file only once git status has loaded and lists no conflicted file.
const noneLeft = computed(() => !!status.value && conflicted.value.length === 0)

// The tab lasts the whole session, so it never rests on a file that is no longer conflicted,
// whether it was applied here, resolved by the model or settled outside the app: it moves to the
// file that followed it in the list as last seen, or the first one.
let lastSeen: string[] = []
watch(
  [conflicted, () => props.path],
  ([paths, path]) => {
    if (!status.value) return
    if (paths.length > 0 && !paths.includes(path)) {
      const at = lastSeen.indexOf(path)
      const next = at === -1 ? undefined : lastSeen.slice(at + 1).find((p) => paths.includes(p))
      setConflictPath(props.project, props.tabId, next ?? paths[0])
    }
    lastSeen = paths
  },
  { immediate: true }
)
</script>

<template>
  <!-- Each file the tab showed stays cached, keyed by path, so switching files keeps its unsaved
       middle and its undo history, as switching tabs does. -->
  <div v-if="noneLeft" class="none-left">
    No conflicts left in this project. You can close this tab.
  </div>
  <KeepAlive v-else>
    <ConflictFileView :key="path" :project="project" :path="path" :thread="thread">
      <template #picker>
        <ConflictFilePicker
          :project="project"
          :path="path"
          :thread="thread"
          @select="setConflictPath(project, tabId, $event)"
        />
      </template>
    </ConflictFileView>
  </KeepAlive>
</template>

<style scoped>
.none-left {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  padding: var(--space-6);
  background: var(--color-surface-0);
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  text-align: center;
}
</style>

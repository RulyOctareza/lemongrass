<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
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
</script>

<template>
  <!-- Each file the tab showed stays cached, keyed by path, so switching files keeps its unsaved
       middle and its undo history, as switching tabs does. -->
  <KeepAlive>
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

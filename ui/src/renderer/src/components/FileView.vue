<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import AppEmptyState from './AppEmptyState.vue'
import ReadOnlyFile from './ReadOnlyFile.vue'
import { loadProjectFile, workspaceOf, type ProjectRef } from '../workspace'
import type { WorkspaceTab } from '../../../preload/types'

type FileTab = Extract<WorkspaceTab, { kind: 'file' }>

const props = defineProps<{
  project: ProjectRef
  tab: FileTab
}>()

const entry = computed(() => workspaceOf(props.project.id)?.files[props.tab.path])

const imageSrc = computed(() => {
  const query = new URLSearchParams({
    p: props.project.path,
    f: props.tab.path,
    v: String(entry.value?.imageVersion ?? 0)
  })
  return `project-file://project/?${query}`
})

// The body unmounts in the background, so every show rereads the file: a file whose folder the
// explorer does not watch is still current when looked at.
onMounted(() => void loadProjectFile(props.project, props.tab.path, true))
</script>

<template>
  <div class="file-view">
    <AppEmptyState
      v-if="!entry || entry.status === 'loading'"
      layout="fill"
      loading
      title="Loading..."
    />
    <AppEmptyState
      v-else-if="entry.status === 'missing'"
      layout="fill"
      title="This file no longer exists."
      detail="Close the tab to remove it."
    />
    <ReadOnlyFile v-else :entry="entry" :path="tab.path" :image-src="imageSrc" :alt="tab.path" />
  </div>
</template>

<style scoped>
.file-view {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
</style>

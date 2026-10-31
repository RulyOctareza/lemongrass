<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import { gitStateOf } from '../../git'
import type { ConflictThreadState } from '../../conflictThread'
import { fileStatus, fileStatusText } from '../../conflictThreadModel'
import type { ProjectRef } from '../../workspace'
import AppSwitcher from '../AppSwitcher.vue'

// Switches the file the conflict tab shows, among the files git still reports conflicted.
const props = defineProps<{
  project: ProjectRef
  path: string
  thread: ConflictThreadState
}>()

const emit = defineEmits<{ select: [path: string] }>()

// Past this many files the list gets a filter box.
const SEARCH_FROM = 8

function nameOf(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

function blocksText(blocks: number): string {
  return blocks === 1 ? '1 conflict' : `${blocks} conflicts`
}

const items = computed(() =>
  (gitStateOf(props.project.id).status?.files ?? [])
    .filter((file) => file.group === 'conflicted')
    .map((file) => {
      const status = fileStatus(props.thread.messages, file.path, file.conflictBlocks)
      return {
        id: file.path,
        label: nameOf(file.path),
        title: file.path,
        search: file.path,
        hint: status
          ? fileStatusText(status)
          : file.conflictBlocks
            ? blocksText(file.conflictBlocks)
            : undefined
      }
    })
)
</script>

<template>
  <AppSwitcher
    class="file-picker"
    :items="items"
    :active-id="path"
    :label="nameOf(path)"
    :title="path"
    placeholder="Pick a file"
    empty-text="No conflicted files left."
    :searchable="items.length > SEARCH_FROM"
    search-placeholder="Filter files"
    @select="emit('select', $event)"
  />
</template>

<style scoped>
.file-picker {
  flex-shrink: 0;
}
</style>

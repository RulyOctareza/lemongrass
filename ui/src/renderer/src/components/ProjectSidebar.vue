<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppEmptyState from './AppEmptyState.vue'
import AppSegmented from './AppSegmented.vue'
import BiblioPanel from './BiblioPanel.vue'
import DeviceEntry from './DeviceEntry.vue'
import FileExplorer from './explorer/FileExplorer.vue'
import GitPanel from './GitPanel.vue'
import NodeEntry from './NodeEntry.vue'
import ResterClientEntry from './ResterClientEntry.vue'
import VaultEntry from './VaultEntry.vue'
import WorkgroupPanel from './WorkgroupPanel.vue'
import {
  loadSidebarSettings,
  setSidebarView,
  sidebarSettings,
  type SidebarView
} from '../explorerSettings'
import type { BiblioTree } from '../../../preload/types'
import type { ProjectRef } from '../workspace'

defineProps<{
  collapsed: boolean
  project: ProjectRef | undefined
  tree: BiblioTree | null
}>()

const emit = defineEmits<{
  openVault: []
  refreshBiblio: []
}>()

const VIEWS: { value: SidebarView; label: string }[] = [
  { value: 'biblio', label: 'Biblio' },
  { value: 'files', label: 'Files' }
]

onMounted(() => void loadSidebarSettings())

// The tree column stays nested in the sidebar with the width on the column itself, so the sidebar
// takes its natural width from it. As a sibling of the sidebar it shows a dead gap beside the tree
// under electron-vite dev.
const treeWidth = ref(240)
let dragStartX = 0
let dragStartWidth = 0

function onGutterDown(event: PointerEvent): void {
  dragStartX = event.clientX
  dragStartWidth = treeWidth.value
  window.addEventListener('pointermove', onGutterMove)
  window.addEventListener('pointerup', onGutterUp)
}

function onGutterMove(event: PointerEvent): void {
  const delta = event.clientX - dragStartX
  treeWidth.value = Math.min(480, Math.max(180, dragStartWidth + delta))
}

function onGutterUp(): void {
  window.removeEventListener('pointermove', onGutterMove)
  window.removeEventListener('pointerup', onGutterUp)
}
</script>

<template>
  <div class="sidebar" :class="{ collapsed }">
    <div class="sidebar-main">
      <div v-if="project" class="sidebar-pane">
        <div class="sidebar-tree" :style="{ width: `${treeWidth}px` }">
          <AppSegmented
            class="view-switch"
            aria-label="Sidebar view"
            :options="VIEWS"
            :model-value="sidebarSettings.view"
            @update:model-value="setSidebarView"
          />
          <BiblioPanel
            v-if="sidebarSettings.view === 'biblio'"
            :tree="tree"
            :project="project"
            @refresh="emit('refreshBiblio')"
          />
          <FileExplorer v-else :project="project" />

          <WorkgroupPanel :project="project" />
          <GitPanel :project="project" />
          <VaultEntry @open="emit('openVault')" />
          <DeviceEntry :project="project" />
          <ResterClientEntry :project="project" />
          <NodeEntry />
        </div>

        <div class="sidebar-gutter" @pointerdown="onGutterDown" />
      </div>
      <AppEmptyState v-else layout="fill" title="No project selected." />
    </div>
  </div>
</template>

<style scoped>
.sidebar {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-1);
  overflow: hidden;
  transition: width var(--duration-base) var(--ease-out);
}

.sidebar.collapsed {
  width: 0;
}

.sidebar-main {
  flex: 1;
  min-height: 0;
  display: flex;
}

.sidebar-pane {
  flex-shrink: 0;
  min-height: 0;
  display: flex;
}

.sidebar-tree {
  flex-shrink: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-1);
}

.sidebar-tree .view-switch {
  align-self: stretch;
  margin: var(--space-3) var(--space-2) 0;
}

.sidebar-tree .view-switch :deep(button) {
  flex: 1;
}

.sidebar-gutter {
  flex-shrink: 0;
  width: 4px;
  cursor: col-resize;
  background: var(--color-border-subtle);
  transition: background var(--duration-fast) var(--ease-out);
}

.sidebar-gutter:hover,
.sidebar-gutter:active {
  background: var(--color-amber);
}
</style>

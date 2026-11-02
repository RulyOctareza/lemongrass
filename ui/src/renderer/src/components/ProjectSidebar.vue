<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import BiblioPanel from './BiblioPanel.vue'
import type { BiblioTree } from '../../../preload/types'
import type { ProjectRef } from '../workspace'
import AppEmptyState from './AppEmptyState.vue'

defineProps<{
  collapsed: boolean
  project: ProjectRef | undefined
  tree: BiblioTree | null
}>()

const emit = defineEmits<{
  openVault: []
  refreshBiblio: []
}>()
</script>

<template>
  <div class="sidebar" :class="{ collapsed }">
    <div class="sidebar-main">
      <BiblioPanel
        v-if="project"
        :tree="tree"
        :project="project"
        @refresh="emit('refreshBiblio')"
        @open-vault="emit('openVault')"
      />
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
</style>

<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import BiblioPanel from './BiblioPanel.vue'
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
      <p v-else class="empty">No project selected.</p>
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

.empty {
  padding: var(--space-4) var(--space-3);
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}
</style>

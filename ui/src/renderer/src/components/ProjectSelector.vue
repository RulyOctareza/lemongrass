<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import AppButton from './AppButton.vue'
import type { Project } from '../../../preload/types'
import AppEmptyState from './AppEmptyState.vue'

defineProps<{
  projects: Project[]
}>()

const emit = defineEmits<{
  select: [id: string]
  add: []
}>()
</script>

<template>
  <div class="selector">
    <div class="selector-card">
      <h2 class="selector-title">Open a project</h2>
      <AppEmptyState v-if="projects.length === 0" title="Add a project to get started." />
      <ul v-else class="selector-list">
        <li v-for="project in projects" :key="project.id">
          <button type="button" class="selector-row" @click="emit('select', project.id)">
            <span class="selector-name">{{ project.name }}</span>
            <span class="selector-path">{{ project.path }}</span>
          </button>
        </li>
      </ul>
      <AppButton type="button" class="selector-add" @click="emit('add')">Add project</AppButton>
    </div>
  </div>
</template>

<style scoped>
.selector {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-6);
  overflow: auto;
}

.selector-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  width: 100%;
  max-width: 32rem;
  max-height: 100%;
}

.selector-title {
  color: var(--color-fg-primary);
  font-family: var(--font-display);
  font-size: var(--text-lg);
  font-weight: var(--weight-bold);
  letter-spacing: var(--tracking-snug);
}

.selector-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
  overflow-y: auto;
}

.selector-row {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  width: 100%;
  padding: var(--space-3) var(--space-4);
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  text-align: left;
  cursor: pointer;
}

.selector-row:hover,
.selector-row:focus-visible {
  outline: none;
  border-color: var(--color-amber);
}

.selector-name {
  font-size: var(--text-sm);
  font-weight: var(--weight-medium);
}

.selector-path {
  max-width: 100%;
  overflow: hidden;
  color: var(--color-fg-muted);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.selector-add {
  align-self: flex-start;
}
</style>

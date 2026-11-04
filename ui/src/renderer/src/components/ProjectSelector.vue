<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import AppButton from './AppButton.vue'
import AppConfirmButton from './AppConfirmButton.vue'
import AppIcon from './AppIcon.vue'
import type { Project } from '../../../preload/types'
import AppEmptyState from './AppEmptyState.vue'
import { useProjectDelete } from '../projectDelete'

const props = defineProps<{
  projects: Project[]
}>()

const emit = defineEmits<{
  select: [id: string]
  add: []
  deleted: [id: string]
}>()

// The page also shows right after the active project is deleted, while other projects may still
// have tabs running, so the shells are reread whenever the list changes.
const { refreshLiveShells, refusal, remove: removeProject } = useProjectDelete()
onMounted(refreshLiveShells)
watch(() => props.projects, refreshLiveShells)

async function remove(id: string): Promise<void> {
  if (await removeProject(id)) emit('deleted', id)
}
</script>

<template>
  <div class="selector">
    <div class="selector-card">
      <h2 class="selector-title">Open a project</h2>
      <AppEmptyState v-if="projects.length === 0" title="Add a project to get started." />
      <ul v-else class="selector-list">
        <li v-for="project in projects" :key="project.id" class="selector-row">
          <button type="button" class="selector-pick" @click="emit('select', project.id)">
            <span class="selector-name">{{ project.name }}</span>
            <span class="selector-path">{{ project.path }}</span>
          </button>
          <span v-if="refusal(project.id)" class="delete-slot" :title="refusal(project.id)">
            <button type="button" class="delete" disabled :aria-label="refusal(project.id)">
              <AppIcon name="trash" :size="18" />
            </button>
          </span>
          <AppConfirmButton
            v-else
            class="delete"
            title="Delete project"
            aria-label="Delete project"
            confirm-label="Click again to delete. The folder is not touched, and lemongrass keeps its data for 30 days."
            @confirm="remove(project.id)"
          >
            <AppIcon name="trash" :size="18" />
          </AppConfirmButton>
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
  align-items: center;
  gap: var(--space-2);
  padding-right: var(--space-3);
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
}

.selector-row:hover,
.selector-row:focus-within {
  border-color: var(--color-amber);
}

.selector-pick {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: var(--space-3) var(--space-4);
  background: transparent;
  border: none;
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  text-align: left;
  cursor: pointer;
}

.selector-pick:focus-visible {
  outline: none;
}

.delete-slot {
  display: inline-flex;
}

/* Shown only on the row under the pointer or holding focus; hidden keeps its space, so the path's
   width never jumps. Padding gives the confirm's progress bar and armed background room. */
.delete {
  flex-shrink: 0;
  display: inline-flex;
  padding: 5px;
  background: none;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  visibility: hidden;
  cursor: pointer;
  transition: color var(--duration-fast) var(--ease-out);
}

.selector-row:hover .delete,
.selector-row:focus-within .delete {
  visibility: visible;
}

.delete:hover:not(:disabled),
.delete:focus-visible:not(:disabled) {
  color: var(--color-error);
}

.selector-row .delete:disabled {
  opacity: 0.4;
  cursor: default;
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

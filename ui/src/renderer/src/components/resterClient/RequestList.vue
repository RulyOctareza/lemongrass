<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import AppConfirmButton from '../AppConfirmButton.vue'
import type { ResterClientRequest } from '../../../../preload/types'
import AppInput from '../AppInput.vue'
import type { AppInputHandle } from '../appInput'

defineProps<{
  requests: ResterClientRequest[]
  selectedId: string | null
}>()

const emit = defineEmits<{
  select: [id: string]
  remove: [id: string]
  rename: [id: string, name: string]
}>()

const renamingId = ref<string | null>(null)
const renameText = ref('')
const renameInput = ref<AppInputHandle[] | null>(null)

function title(request: ResterClientRequest): string {
  return request.name || request.url || 'Untitled request'
}

async function startRename(request: ResterClientRequest): Promise<void> {
  renamingId.value = request.id
  renameText.value = request.name
  await nextTick()
  renameInput.value?.[0]?.select()
}

function finishRename(): void {
  if (renamingId.value) emit('rename', renamingId.value, renameText.value.trim())
  renamingId.value = null
}
</script>

<template>
  <div class="request-list">
    <p v-if="requests.length === 0" class="hint">No requests yet.</p>
    <div
      v-for="request in requests"
      :key="request.id"
      class="request-item"
      :class="{ active: request.id === selectedId }"
      @click="emit('select', request.id)"
      @dblclick="startRename(request)"
    >
      <span class="method" :class="request.method.toLowerCase()">{{ request.method }}</span>
      <AppInput
        v-if="renamingId === request.id"
        ref="renameInput"
        v-model="renameText"
        size="sm"
        class="rename"
        type="text"
        placeholder="Request name"
        @click.stop
        @keydown.enter="finishRename"
        @keydown.escape="renamingId = null"
        @blur="finishRename"
      />
      <span v-else class="name" :title="`${title(request)} (double-click to rename)`">
        {{ title(request) }}
      </span>
      <AppConfirmButton
        class="remove"
        title="Delete request"
        @click.stop
        @confirm="emit('remove', request.id)"
      >
        ×
      </AppConfirmButton>
    </div>
  </div>
</template>

<style scoped>
.request-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.request-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  cursor: pointer;
}

.request-item:hover {
  background: var(--color-surface-2);
}

.request-item.active {
  background: var(--color-surface-3);
  color: var(--color-fg-primary);
}

.method {
  flex-shrink: 0;
  width: 44px;
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: var(--weight-bold);
  color: var(--color-fg-muted);
}

.method.get {
  color: var(--color-jade);
}

.method.post {
  color: var(--color-amber);
}

.method.put,
.method.patch {
  color: var(--color-sapphire);
}

.method.delete {
  color: var(--color-coral);
}

.name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rename {
  flex: 1;
  min-width: 0;
}

.remove {
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  cursor: pointer;
  visibility: hidden;
}

.request-item:hover .remove {
  visibility: visible;
}

.remove:hover {
  background: var(--color-surface-3);
  color: var(--color-fg-primary);
}
</style>

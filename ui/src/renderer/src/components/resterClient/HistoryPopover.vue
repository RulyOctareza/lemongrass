<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import AppIcon from '../AppIcon.vue'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import AppConfirmButton from '../AppConfirmButton.vue'
import RunHistory from './RunHistory.vue'
import { formatAge } from '../../relativeAge'
import { ageNow } from '../../ageClock'
import type { ResterClientRun } from '../../../../preload/types'

defineProps<{
  // When the shown response came back.
  at: string
  runs: ResterClientRun[]
  runId: string | null
}>()

const emit = defineEmits<{
  pick: [run: ResterClientRun]
  remove: [runId: string]
  clear: []
}>()

const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)

function pick(run: ResterClientRun): void {
  emit('pick', run)
  open.value = false
}

function onDocumentClick(event: MouseEvent): void {
  if (open.value && rootRef.value && !rootRef.value.contains(event.target as Node)) {
    open.value = false
  }
}

function onDocumentKeydown(event: KeyboardEvent): void {
  if (open.value && event.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick, true)
  document.addEventListener('keydown', onDocumentKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick, true)
  document.removeEventListener('keydown', onDocumentKeydown)
})
</script>

<template>
  <div ref="rootRef" class="history">
    <button
      type="button"
      class="age"
      :class="{ open }"
      :title="`${new Date(at).toLocaleString()}. Click for this request's history.`"
      @click="open = !open"
    >
      {{ formatAge(at, ageNow) }}
      <AppIcon class="caret" name="chevronDown" :size="9" />
    </button>
    <div v-if="open" class="popover">
      <div class="popover-head">
        <span class="popover-title">
          {{ runs.length }} {{ runs.length === 1 ? 'run' : 'runs' }}
        </span>
        <AppConfirmButton
          v-if="runs.length > 0"
          variant="ghost"
          confirm-label="Click again to clear"
          @confirm="emit('clear')"
        >
          Clear
        </AppConfirmButton>
      </div>
      <div class="popover-list">
        <RunHistory
          :runs="runs"
          :selected-run-id="runId"
          @pick="pick"
          @remove="emit('remove', $event)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.history {
  position: relative;
}

.age {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  white-space: nowrap;
  cursor: pointer;
}

.age:hover,
.age.open {
  background: var(--color-surface-3);
  border-color: var(--color-border-default);
  color: var(--color-fg-primary);
}

.caret {
  transition: transform var(--duration-fast) var(--ease-out);
}

.age.open .caret {
  transform: rotate(180deg);
}

.popover {
  position: absolute;
  top: calc(100% + var(--space-2));
  right: 0;
  z-index: 80;
  width: 320px;
  display: flex;
  flex-direction: column;
  background: var(--layer-float);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-xl);
  overflow: hidden;
}

.popover-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border-default);
}

.popover-title {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.popover-list {
  max-height: 360px;
  overflow-y: auto;
  padding: var(--space-2);
}
</style>

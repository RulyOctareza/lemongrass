<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

export interface MenuItem {
  key: string
  label: string
  disabled?: boolean
  title?: string
  // A destructive item turns red with a "Click again" hint on its first click and is selected
  // only on the second, the way the tab menu's close actions behave.
  confirm?: boolean
}

const props = defineProps<{
  x: number
  y: number
  items: MenuItem[]
}>()

const emit = defineEmits<{
  select: [key: string]
  close: []
}>()

const confirmingKey = ref<string | null>(null)

function choose(item: MenuItem): void {
  if (item.confirm && confirmingKey.value !== item.key) {
    confirmingKey.value = item.key
    return
  }
  confirmingKey.value = null
  emit('select', item.key)
}

const WIDTH = 200
const ROW = 30

// Keeps the menu inside the window when opened near the right or bottom edge.
const left = computed(() => Math.max(0, Math.min(props.x, window.innerWidth - WIDTH - 8)))
const top = computed(() =>
  Math.max(0, Math.min(props.y, window.innerHeight - props.items.length * ROW - 16))
)

function onPointerDown(event: PointerEvent): void {
  if (!(event.target as HTMLElement).closest('.context-menu')) emit('close')
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') emit('close')
}

onMounted(() => {
  window.addEventListener('pointerdown', onPointerDown, true)
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onPointerDown, true)
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="context-menu" role="menu" :style="{ left: `${left}px`, top: `${top}px` }">
    <button
      v-for="item in items"
      :key="item.key"
      class="menu-item"
      role="menuitem"
      :class="{ confirming: confirmingKey === item.key }"
      :disabled="item.disabled"
      :title="item.title"
      @click="choose(item)"
    >
      {{ item.label }}
      <span v-if="item.confirm" class="menu-confirm-hint">Click again</span>
    </button>
  </div>
</template>

<style scoped>
.context-menu {
  position: fixed;
  z-index: 50;
  min-width: 180px;
  padding: var(--space-2);
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg, 0 8px 24px rgba(0, 0, 0, 0.4));
}

.menu-item {
  display: flex;
  align-items: center;
  width: 100%;
  padding: var(--space-1) var(--space-3);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
}

/* The menu sits on --color-bg-elevated, which equals --color-surface-2 in the dark theme. */
.menu-item:hover:not(:disabled),
.menu-item:focus-visible:not(:disabled) {
  background: var(--color-surface-3);
}

.menu-item.confirming,
.menu-item.confirming:hover:not(:disabled) {
  background: var(--color-error-muted);
  color: var(--color-error);
}

.menu-confirm-hint {
  margin-left: auto;
  padding-left: var(--space-4);
  font-size: var(--text-xs);
  visibility: hidden;
}

.menu-item.confirming .menu-confirm-hint {
  visibility: visible;
}

.menu-item:disabled {
  color: var(--color-fg-muted);
  cursor: default;
}
</style>

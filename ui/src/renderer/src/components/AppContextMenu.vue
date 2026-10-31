<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'
import AppConfirmButton from './AppConfirmButton.vue'

export interface MenuItem {
  key: string
  label: string
  disabled?: boolean
  title?: string
  // A destructive item is an AppConfirmButton, selected only on its armed click.
  confirm?: boolean
  // Stroke path data in a 14 by 14 box, drawn before the label.
  icon?: readonly string[]
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
    <template v-for="item in items" :key="item.key">
      <AppConfirmButton
        v-if="item.confirm"
        v-slot="{ armed }"
        class="menu-item"
        role="menuitem"
        :disabled="item.disabled"
        :title="item.title"
        @confirm="emit('select', item.key)"
      >
        <svg
          v-if="item.icon"
          class="menu-icon"
          width="13"
          height="13"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.3"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path v-for="d in item.icon" :key="d" :d="d" />
        </svg>
        {{ item.label }}
        <span class="menu-confirm-hint" :class="{ shown: armed }">Click again</span>
      </AppConfirmButton>
      <button
        v-else
        class="menu-item"
        role="menuitem"
        :disabled="item.disabled"
        :title="item.title"
        @click="emit('select', item.key)"
      >
        <svg
          v-if="item.icon"
          class="menu-icon"
          width="13"
          height="13"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.3"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path v-for="d in item.icon" :key="d" :d="d" />
        </svg>
        {{ item.label }}
      </button>
    </template>
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
  gap: var(--space-2);
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

.menu-icon {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
}

.menu-item:disabled .menu-icon {
  color: inherit;
}

.menu-confirm-hint {
  margin-left: auto;
  padding-left: var(--space-4);
  font-size: var(--text-xs);
  visibility: hidden;
}

.menu-confirm-hint.shown {
  visibility: visible;
}

.menu-item:disabled {
  color: var(--color-fg-muted);
  cursor: default;
}
</style>

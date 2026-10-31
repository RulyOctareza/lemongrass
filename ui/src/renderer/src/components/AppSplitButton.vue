<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { ref } from 'vue'
import AppButton from './AppButton.vue'
import type { MenuAnchor } from '../menuPlacement'
import AppContextMenu, { type MenuItem } from './AppContextMenu.vue'

// A main action with its alternatives behind a chevron: the main part runs the default, the
// chevron opens an AppContextMenu with the rest. Both parts share one pill and one variant.
withDefaults(
  defineProps<{
    variant?: 'primary' | 'ghost' | 'danger'
    disabled?: boolean
    items: MenuItem[]
    menuLabel?: string
  }>(),
  { variant: 'ghost', disabled: false, menuLabel: 'More actions' }
)

const emit = defineEmits<{
  click: [event: MouseEvent]
  select: [key: string]
  // The chevron menu opened, so a caller can close its own popup for the main part.
  menu: []
}>()

const GAP = 4

const root = ref<HTMLElement | null>(null)
const menu = ref<MenuAnchor | null>(null)
// The menu closes on any pointerdown outside it, the chevron included, before the chevron's click
// arrives; this keeps that click from opening it again.
let closedAt = 0

function toggleMenu(): void {
  if (menu.value) {
    menu.value = null
    return
  }
  if (Date.now() - closedAt < 300) return
  const rect = root.value?.getBoundingClientRect()
  if (!rect) return
  // Left-aligned to the control, below it or above.
  menu.value = {
    x: { start: rect.left, end: rect.right },
    y: { start: rect.bottom + GAP, end: rect.top - GAP }
  }
  emit('menu')
}

function closeMenu(): void {
  menu.value = null
  closedAt = Date.now()
}

function select(key: string): void {
  menu.value = null
  emit('select', key)
}

defineExpose({ el: root })
</script>

<template>
  <div ref="root" class="app-split">
    <AppButton
      :variant="variant"
      class="split-main"
      :disabled="disabled"
      @click="emit('click', $event)"
    >
      <span class="split-label"><slot /></span>
    </AppButton>
    <AppButton
      :variant="variant"
      class="split-chevron"
      :disabled="disabled"
      aria-haspopup="menu"
      :aria-expanded="!!menu"
      :title="menuLabel"
      :aria-label="menuLabel"
      @click="toggleMenu"
    >
      <svg
        width="10"
        height="10"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.3"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M2.5 4l2.5 2.5L7.5 4" />
      </svg>
    </AppButton>
    <Teleport to="body">
      <AppContextMenu
        v-if="menu"
        :anchor="menu"
        :items="items"
        @select="select"
        @close="closeMenu"
      />
    </Teleport>
  </div>
</template>

<style scoped>
/* A 1px gap between the parts lets the background show through as the divider. */
.app-split {
  display: inline-flex;
  gap: 1px;
  min-width: 0;
}

.app-button.split-main {
  flex: 1;
  min-width: 0;
  padding-left: var(--space-3);
  padding-right: var(--space-3);
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
}

.split-label {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.app-button.split-chevron {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  padding-left: 6px;
  padding-right: 6px;
  border-top-left-radius: 0;
  border-bottom-left-radius: 0;
}
</style>

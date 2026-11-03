<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { ref } from 'vue'
import TabIcon from '../TabIcon.vue'
import MatchName from './MatchName.vue'
import type { FileIcon } from '../../fileIcons'
import type { MatchRange } from '../../treeSearch'

// One row of a sidebar tree, shared by the Files tree and the search results of both views. The
// root is a div rather than a button so a row can hold its own trailing controls and be dragged.
withDefaults(
  defineProps<{
    depth: number
    name: string
    icon: FileIcon
    dir?: boolean
    expanded?: boolean
    ranges?: MatchRange[]
    active?: boolean
    selected?: boolean
    muted?: boolean
    dropTarget?: boolean
    density?: 'compact' | 'comfortable'
  }>(),
  {
    dir: false,
    expanded: false,
    ranges: undefined,
    active: false,
    selected: false,
    muted: false,
    dropTarget: false,
    density: 'compact'
  }
)

const root = ref<HTMLElement | null>(null)
defineExpose({ focus: () => root.value?.focus({ preventScroll: false }), el: root })
</script>

<template>
  <div
    ref="root"
    class="tree-row"
    :class="[density, { active, selected, muted, 'drop-target': dropTarget }]"
    role="treeitem"
    tabindex="-1"
    :aria-expanded="dir ? expanded : undefined"
    :aria-selected="selected"
    :style="{ paddingLeft: `${depth * 14 + 10}px` }"
  >
    <svg
      v-if="dir"
      class="chevron"
      :class="{ expanded }"
      width="9"
      height="9"
      viewBox="0 0 9 9"
      fill="none"
      stroke="currentColor"
      stroke-width="1.4"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M2 1.5 6.5 4.5 2 7.5" />
    </svg>
    <span v-else class="chevron" />
    <TabIcon class="icon" :shapes="icon.shapes" :style="{ color: icon.color }" />
    <MatchName class="label" :text="name" :ranges="ranges" />
    <slot />
  </div>
</template>

<style scoped>
.tree-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  padding-right: var(--space-2);
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  cursor: pointer;
  user-select: none;
  outline: none;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.tree-row.compact {
  padding-top: 3px;
  padding-bottom: 3px;
}

.tree-row.comfortable {
  padding-top: 6px;
  padding-bottom: 6px;
}

.tree-row:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.tree-row.selected {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
  box-shadow: inset 0 0 0 1px var(--color-border-strong);
}

.tree-row.active {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
  font-weight: var(--weight-medium);
}

.tree-row.drop-target {
  background: var(--color-amber-muted);
  box-shadow: inset 0 0 0 1px var(--color-amber);
}

.tree-row.muted {
  opacity: 0.55;
}

.chevron {
  flex-shrink: 0;
  width: 9px;
  color: var(--color-fg-muted);
  transition: transform var(--duration-fast) var(--ease-out);
}

.chevron.expanded {
  transform: rotate(90deg);
}

.icon {
  flex-shrink: 0;
}

.label {
  flex: 1;
}
</style>

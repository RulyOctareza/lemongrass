<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import TabIcon from './TabIcon.vue'
import { archiveIcon, biblioDirShapes, biblioFileShapes } from '../biblioIcons'
import type { BiblioNode } from '../../../preload/types'

// The expanded folders live in the panel, by path, so a search result can open the folders above
// it and the tree keeps its shape across a switch to the Files view and back.
const props = defineProps<{
  node: BiblioNode
  selectedPath: string | null
  depth: number
  expandedPaths: Record<string, true>
}>()

const emit = defineEmits<{
  select: [path: string]
  toggle: [path: string]
  expand: [path: string]
  'add-scratchpad': []
  'add-scratchpad-file': [folderPath: string]
  'archive-scratchpad': [taskPath: string]
}>()

const expanded = computed((): boolean => !!props.expandedPaths[props.node.path])

const isArchive = computed((): boolean => props.node.path === 'scratchpad/archive')

const isTopLevelScratchpad = computed(
  (): boolean => props.depth === 0 && props.node.name === 'scratchpad'
)

// Covers the top-level scratchpad row and every folder nested under scratchpad/, except the archive.
const showAddButton = computed(
  (): boolean =>
    props.node.type === 'dir' &&
    props.node.path !== 'scratchpad/archive' &&
    !props.node.path.startsWith('scratchpad/archive/') &&
    (isTopLevelScratchpad.value ||
      props.node.path === 'scratchpad' ||
      props.node.path.startsWith('scratchpad/'))
)

function onAddClick(): void {
  emit('expand', props.node.path)
  if (isTopLevelScratchpad.value) {
    emit('add-scratchpad')
  } else {
    emit('add-scratchpad-file', props.node.path)
  }
}

// A task folder directly under scratchpad/ (e.g. "scratchpad/some-task"), excluding
// the archive folder itself -- that's the one row this button is offered on.
const isArchivableScratchpadTask = computed((): boolean => {
  if (props.node.type !== 'dir') return false
  const segments = props.node.path.split('/')
  return segments.length === 2 && segments[0] === 'scratchpad' && segments[1] !== 'archive'
})
</script>

<template>
  <button
    v-if="node.type === 'file'"
    class="tree-item file"
    :class="{ active: node.path === selectedPath }"
    :style="{ paddingLeft: `${depth * 14 + 10}px` }"
    @click="emit('select', node.path)"
  >
    <span class="indent" />
    <TabIcon class="icon" :shapes="biblioFileShapes(node.name)" />
    <span class="label">{{ node.name.replace(/\.md$/, '') }}</span>
  </button>

  <div
    v-else
    class="dir-row"
    :class="{ 'archive-row': isArchive }"
    :style="isArchive ? { marginLeft: `${depth * 14}px`, width: 'auto' } : undefined"
  >
    <button
      class="tree-item tree-item-main"
      :style="isArchive ? undefined : { paddingLeft: `${depth * 14 + 10}px` }"
      @click="emit('toggle', node.path)"
    >
      <svg
        class="indent chevron"
        :class="{ expanded }"
        width="9"
        height="9"
        viewBox="0 0 9 9"
        fill="none"
        stroke="currentColor"
        stroke-width="1.4"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M2 1.5 6.5 4.5 2 7.5" />
      </svg>

      <TabIcon class="icon" :shapes="biblioDirShapes(node.path)" />

      <span class="label">{{ node.name }}</span>
    </button>
    <button
      v-if="isArchivableScratchpadTask"
      class="tree-item-add"
      title="Archive"
      @click.stop="emit('archive-scratchpad', node.path)"
    >
      <TabIcon :shapes="archiveIcon" class="small-icon" />
    </button>
    <button
      v-if="showAddButton"
      class="tree-item-add"
      :title="isTopLevelScratchpad ? 'Add scratchpad' : 'Add file'"
      @click.stop="onAddClick"
    >
      <svg
        width="10"
        height="10"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.3"
        stroke-linecap="round"
      >
        <line x1="5" y1="1" x2="5" y2="9" />
        <line x1="1" y1="5" x2="9" y2="5" />
      </svg>
    </button>
  </div>

  <template v-if="node.children && expanded">
    <BiblioTreeItem
      v-for="child in node.children"
      :key="child.path"
      :node="child"
      :selected-path="selectedPath"
      :depth="depth + 1"
      :expanded-paths="expandedPaths"
      @select="(path) => emit('select', path)"
      @toggle="(path) => emit('toggle', path)"
      @expand="(path) => emit('expand', path)"
      @add-scratchpad="emit('add-scratchpad')"
      @add-scratchpad-file="(folderPath) => emit('add-scratchpad-file', folderPath)"
      @archive-scratchpad="(taskPath) => emit('archive-scratchpad', taskPath)"
    />
  </template>
</template>

<style scoped>
.archive-row {
  margin-top: var(--space-1);
  margin-bottom: var(--space-1);
  margin-right: var(--space-2);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  transition: background var(--duration-fast) var(--ease-out);
}

.archive-row:hover {
  background: var(--color-surface-3);
}

.archive-row .tree-item-main {
  padding-left: var(--space-3);
  color: var(--color-fg-primary);
  font-weight: var(--weight-semibold);
}

.archive-row .tree-item:hover {
  background: transparent;
}

.tree-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  text-align: left;
  padding-top: var(--space-1);
  padding-bottom: var(--space-1);
  padding-right: var(--space-3);
  background: transparent;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  cursor: pointer;
  border-radius: var(--radius-md);
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.tree-item:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.tree-item.file.active {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
  font-weight: var(--weight-medium);
}

.dir-row {
  display: flex;
  align-items: center;
  width: 100%;
  min-width: 0;
}

.tree-item-main {
  flex: 1;
  color: var(--color-fg-secondary);
  font-weight: var(--weight-medium);
}

.tree-item-add {
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  margin-right: var(--space-2);
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  border-radius: var(--radius-pill);
  color: var(--color-fg-muted);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.tree-item-add:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-accent);
}

.indent {
  flex-shrink: 0;
  width: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.chevron {
  color: var(--color-fg-muted);
  transition: transform var(--duration-fast) var(--ease-out);
}

.chevron.expanded {
  transform: rotate(90deg);
}

.small-icon {
  width: 11px;
  height: 11px;
}

.icon {
  flex-shrink: 0;
  color: var(--color-fg-muted);
}

.tree-item-main .icon {
  color: var(--color-fg-secondary);
}

.label {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { BiblioNode } from '../../../preload/types'

const props = defineProps<{
  node: BiblioNode
  selectedPath: string | null
  depth: number
}>()

const emit = defineEmits<{
  select: [path: string]
  'add-scratchpad': []
  'add-scratchpad-file': [folderPath: string]
  'archive-scratchpad': [taskPath: string]
}>()

const expanded = ref(false)

// Only the four top-level category dirs get a distinct icon -- anything
// nested (a book's own folder, handover/archive) reads as a plain folder.
const CATEGORY_ICONS = new Set(['books', 'handover', 'laws', 'scratchpad'])

const iconName = computed((): string => {
  if (props.node.type === 'file') return 'file'
  if (props.depth === 0 && CATEGORY_ICONS.has(props.node.name)) return props.node.name
  if (props.node.path === 'scratchpad/archive') return 'archive'
  return 'folder'
})

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
  expanded.value = true
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

const IMAGE_FILE = /\.(png|jpe?g|gif|webp)$/i

const fileIcon = computed((): string => {
  if (props.node.type !== 'file' || props.node.name.endsWith('.md')) return 'page'
  return IMAGE_FILE.test(props.node.name) ? 'image' : 'code'
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
    <svg
      class="icon"
      width="13"
      height="13"
      viewBox="0 0 14 14"
      fill="none"
      stroke="currentColor"
      stroke-width="1.2"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <template v-if="fileIcon === 'image'">
        <rect x="1.5" y="2.5" width="11" height="9" rx="1" />
        <circle cx="5" cy="5.5" r="1" />
        <path d="M1.5 10l3.5-3 2.5 2 2-1.5 3 2.5" />
      </template>
      <template v-else>
        <path d="M3 1.5h5l3 3v8H3z" />
        <path d="M8 1.5v3h3" />
        <path v-if="fileIcon === 'code'" d="M6 7.5 4.75 9 6 10.5M8 7.5 9.25 9 8 10.5" />
      </template>
    </svg>
    <span class="label">{{ node.name.replace(/\.md$/, '') }}</span>
  </button>

  <div
    v-else
    class="dir-row"
    :class="{ 'archive-row': iconName === 'archive' }"
    :style="iconName === 'archive' ? { marginLeft: `${depth * 14}px`, width: 'auto' } : undefined"
  >
    <button
      class="tree-item tree-item-main"
      :style="iconName === 'archive' ? undefined : { paddingLeft: `${depth * 14 + 10}px` }"
      @click="expanded = !expanded"
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

      <svg
        v-if="iconName === 'books'"
        class="icon"
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <rect x="2" y="1.5" width="10" height="11" rx="1" />
        <line x1="5" y1="1.5" x2="5" y2="12.5" />
      </svg>
      <svg
        v-else-if="iconName === 'laws'"
        class="icon"
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M7 1 12 3 12 7 7 13 2 7 2 3 Z" />
      </svg>
      <svg
        v-else-if="iconName === 'handover'"
        class="icon"
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M1.5 4.5h9M8 2.5l2.5 2-2.5 2" />
        <path d="M12.5 9.5h-9M6 11.5l-2.5-2 2.5-2" />
      </svg>
      <svg
        v-else-if="iconName === 'archive'"
        class="icon"
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <rect x="1.5" y="2" width="11" height="3" rx="0.5" />
        <path d="M2.5 5v6a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1V5" />
        <line x1="5.5" y1="7.5" x2="8.5" y2="7.5" />
      </svg>
      <svg
        v-else-if="iconName === 'scratchpad'"
        class="icon"
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M9.5 1.5 12.5 4.5 5 12H2V9z" />
      </svg>
      <svg
        v-else
        class="icon"
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M1.5 3.5h4l1.5 2h5.5v6h-11z" />
      </svg>

      <span class="label">{{ node.name }}</span>
    </button>
    <button
      v-if="isArchivableScratchpadTask"
      class="tree-item-add"
      title="Archive"
      @click.stop="emit('archive-scratchpad', node.path)"
    >
      <svg
        width="11"
        height="11"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <rect x="1.5" y="2" width="11" height="3" rx="0.5" />
        <path d="M2.5 5v6a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1V5" />
        <line x1="5.5" y1="7.5" x2="8.5" y2="7.5" />
      </svg>
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
      @select="(path) => emit('select', path)"
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

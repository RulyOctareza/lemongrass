<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { ref } from 'vue'
import WorkspacePane from './WorkspacePane.vue'
import { resizeSplit, type ProjectRef } from '../workspace'
import type { WorkspaceLayoutNode } from '../../../preload/types'

const props = defineProps<{
  node: WorkspaceLayoutNode
  project: ProjectRef
  focusedPaneId: string | null
}>()

const MIN_FRACTION = 0.1

const containerEl = ref<HTMLDivElement>()
let dragIndex = -1
let dragStartPos = 0
let dragStartSizes: number[] = []

function onGutterDown(index: number, event: PointerEvent): void {
  if (props.node.type !== 'split') return
  dragIndex = index
  dragStartPos = props.node.direction === 'row' ? event.clientX : event.clientY
  dragStartSizes = [...props.node.sizes]
  window.addEventListener('pointermove', onGutterMove)
  window.addEventListener('pointerup', onGutterUp)
}

function onGutterMove(event: PointerEvent): void {
  const node = props.node
  if (node.type !== 'split' || !containerEl.value) return
  const rect = containerEl.value.getBoundingClientRect()
  const total = node.direction === 'row' ? rect.width : rect.height
  const pos = node.direction === 'row' ? event.clientX : event.clientY
  const delta = (pos - dragStartPos) / total

  const a = dragIndex
  const b = dragIndex + 1
  const pairTotal = dragStartSizes[a] + dragStartSizes[b]
  const min = Math.min(MIN_FRACTION, pairTotal / 2)
  const nextA = Math.max(min, Math.min(pairTotal - min, dragStartSizes[a] + delta))

  const sizes = [...dragStartSizes]
  sizes[a] = nextA
  sizes[b] = pairTotal - nextA
  resizeSplit(props.project, node.id, sizes)
}

function onGutterUp(): void {
  dragIndex = -1
  window.removeEventListener('pointermove', onGutterMove)
  window.removeEventListener('pointerup', onGutterUp)
}
</script>

<template>
  <WorkspacePane
    v-if="node.type === 'leaf'"
    :leaf="node"
    :project="project"
    :focused="node.id === focusedPaneId"
  />

  <div v-else ref="containerEl" class="split" :class="node.direction">
    <template v-for="(child, index) in node.children" :key="child.id">
      <div class="split-child" :style="{ flex: `0 1 ${node.sizes[index] * 100}%` }">
        <WorkspaceLayout :node="child" :project="project" :focused-pane-id="focusedPaneId" />
      </div>
      <div
        v-if="index < node.children.length - 1"
        class="gutter"
        :class="node.direction"
        @pointerdown="onGutterDown(index, $event)"
      />
    </template>
  </div>
</template>

<style scoped>
.split {
  height: 100%;
  width: 100%;
  min-height: 0;
  min-width: 0;
  display: flex;
}

.split.row {
  flex-direction: row;
}

.split.column {
  flex-direction: column;
}

.split-child {
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  display: flex;
}

.gutter {
  flex-shrink: 0;
  background: var(--color-border-subtle);
  transition: background var(--duration-fast) var(--ease-out);
}

.gutter.row {
  width: 4px;
  cursor: col-resize;
}

.gutter.column {
  height: 4px;
  cursor: row-resize;
}

.gutter:hover,
.gutter:active {
  background: var(--color-amber);
}
</style>

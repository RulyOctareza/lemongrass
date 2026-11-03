<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import AppEmptyState from '../AppEmptyState.vue'
import TreeRow from './TreeRow.vue'
import type { FileIcon } from '../../fileIcons'
import type { SearchNode } from '../../treeSearch'
import type { TreeSearch } from '../../useTreeSearch'

// The search results of a sidebar view: the pruned tree from useTreeSearch, every folder open.
// The view supplies its own icons and label rule, so Biblio results keep the biblio look.
const props = defineProps<{
  search: TreeSearch
  iconOf: (node: SearchNode) => FileIcon
  labelOf?: (node: SearchNode) => string
  mutedOf?: (node: SearchNode) => boolean
  activePath: string | null
  density?: 'compact' | 'comfortable'
}>()

const list = ref<HTMLElement | null>(null)

// Keeps the keyboard cursor in view as arrows move it.
watch(
  () => props.search.cursor.value,
  () => {
    void nextTick(() =>
      list.value?.querySelector('.tree-row.selected')?.scrollIntoView({ block: 'nearest' })
    )
  }
)
</script>

<template>
  <div ref="list" class="search-rows" role="tree">
    <template v-if="search.result.value?.rows.length">
      <TreeRow
        v-for="(row, index) in search.result.value.rows"
        :key="row.node.path"
        :depth="row.depth"
        :name="labelOf ? labelOf(row.node) : row.node.name"
        :icon="iconOf(row.node)"
        :dir="row.node.type === 'dir'"
        :expanded="row.node.children.length > 0"
        :ranges="row.node.ranges"
        :active="row.node.path === activePath"
        :selected="index === search.cursor.value"
        :muted="(mutedOf?.(row.node) ?? false) || (!row.node.match && row.node.type === 'dir')"
        :density="density"
        @mousedown.prevent
        @click="search.pick(index)"
      />
    </template>
    <AppEmptyState
      v-else-if="search.result.value"
      size="sm"
      title="No matches."
      detail="Escape clears the search."
    />
  </div>
</template>

<style scoped>
.search-rows {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
</style>

<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import AppInput from '../AppInput.vue'
import type { TreeSearch } from '../../useTreeSearch'

const props = defineProps<{
  search: TreeSearch
  placeholder: string
}>()

const status = computed((): string => {
  const result = props.search.result.value
  if (!result) return ''
  const matches = result.rows.filter((row) => row.node.match).length
  if (result.hidden > 0 || props.search.truncated.value) return `Top ${matches} matches`
  return matches === 1 ? '1 match' : `${matches} matches`
})
</script>

<template>
  <div class="tree-search-bar">
    <AppInput
      :ref="search.bindInput"
      :model-value="search.query.value"
      class="field"
      size="sm"
      icon="search"
      :placeholder="placeholder"
      aria-label="Search the tree"
      @update:model-value="search.setQuery"
      @keydown="search.onInputKeydown"
      @blur="search.onInputBlur"
    />
    <span v-if="status" class="status">{{ status }}</span>
  </div>
</template>

<style scoped>
.tree-search-bar {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: var(--space-2);
}

.tree-search-bar .field {
  width: 100%;
}

.status {
  padding-left: var(--space-1);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}
</style>

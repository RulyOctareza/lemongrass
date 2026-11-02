<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import AppButton from '../AppButton.vue'
import type { ResterClientPair } from '../../../../preload/types'
import AppInput from '../AppInput.vue'

defineProps<{
  addLabel: string
  namePlaceholder: string
}>()

// Edited in place; the parent owns the array.
const pairs = defineModel<ResterClientPair[]>({ required: true })
</script>

<template>
  <div class="pairs">
    <div v-for="(pair, i) in pairs" :key="i" class="pair">
      <AppInput v-model="pair.name" type="text" :placeholder="namePlaceholder" spellcheck="false" />
      <AppInput v-model="pair.value" type="text" placeholder="value" spellcheck="false" />
      <button type="button" class="remove" title="Remove" @click="pairs.splice(i, 1)">×</button>
    </div>
    <AppButton class="add" @click="pairs.push({ name: '', value: '' })">+ {{ addLabel }}</AppButton>
  </div>
</template>

<style scoped>
.pairs {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.pair {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 3fr) auto;
  gap: var(--space-2);
  align-items: center;
}

.remove {
  width: 24px;
  height: 24px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  font-size: var(--text-md);
  cursor: pointer;
}

.remove:hover {
  background: var(--color-surface-3);
  color: var(--color-fg-primary);
}

.add {
  align-self: flex-start;
}
</style>

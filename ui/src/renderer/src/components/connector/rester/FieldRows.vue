<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import AppButton from '../../AppButton.vue'
import AppSegmented from '../../AppSegmented.vue'
import AppSwitcher from '../../AppSwitcher.vue'
import type { FieldDraft, FieldType } from './resterDraft'

defineProps<{
  addLabel: string
}>()

// Edited in place; the parent owns the array, so a row's state follows its index.
const fields = defineModel<FieldDraft[]>({ required: true })

const typeItems: { id: FieldType; label: string }[] = [
  { id: 'text', label: 'Text' },
  { id: 'number', label: 'Number' },
  { id: 'boolean', label: 'Boolean' }
]

const booleanOptions = [
  { value: 'true', label: 'true' },
  { value: 'false', label: 'false' }
]

function setType(field: FieldDraft, type: string): void {
  field.type = type as FieldType
  if (field.type === 'boolean' && field.value !== 'true' && field.value !== 'false') {
    field.value = 'true'
  }
}

function label(type: FieldType): string {
  return typeItems.find((t) => t.id === type)?.label ?? 'Text'
}
</script>

<template>
  <div class="kv-list">
    <div v-for="(field, i) in fields" :key="i" class="kv-row">
      <input v-model="field.key" type="text" placeholder="field name" />
      <div class="kv-type" title="Type sent in the login body">
        <AppSwitcher
          :items="typeItems"
          :active-id="field.type"
          :label="label(field.type)"
          placeholder="Text"
          empty-text=""
          framed
          @select="(id) => setType(field, id)"
        />
      </div>
      <AppSegmented
        v-if="field.type === 'boolean'"
        v-model="field.value"
        class="kv-value"
        :options="booleanOptions"
        :aria-label="`Value of ${field.key || 'field'}`"
      />
      <input
        v-else
        v-model="field.value"
        type="text"
        :inputmode="field.type === 'number' ? 'decimal' : undefined"
        :placeholder="field.type === 'number' ? 'number, e.g. 3' : 'value'"
      />
      <button type="button" class="chip-remove" title="Remove field" @click="fields.splice(i, 1)">
        ×
      </button>
    </div>
    <AppButton
      type="button"
      class="new-channel-button"
      @click="fields.push({ key: '', value: '', type: 'text' })"
    >
      {{ addLabel }}
    </AppButton>
  </div>
</template>

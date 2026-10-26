<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import AppCheckbox from './AppCheckbox.vue'

export interface PickerGroup {
  label?: string
  items: { value: string; label: string }[]
}

const props = withDefaults(
  defineProps<{
    groups: PickerGroup[]
    modelValue: string[]
    summary: string
    filterPlaceholder?: string
    disabled?: boolean
  }>(),
  { filterPlaceholder: 'Filter...', disabled: false }
)

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const open = ref(false)
const filter = ref('')
const root = ref<HTMLElement | null>(null)

const matching = computed(() => {
  const query = filter.value.trim().toLowerCase()
  return props.groups.map((g) => ({
    label: g.label,
    total: g.items.length,
    items: g.items.filter((i) => i.label.toLowerCase().includes(query))
  }))
})

function toggleOpen(): void {
  open.value = !open.value
  if (open.value) filter.value = ''
}

function toggle(value: string): void {
  const current = props.modelValue
  emit(
    'update:modelValue',
    current.includes(value) ? current.filter((v) => v !== value) : [...current, value]
  )
}

// Closes on a click outside the picker. Clicks inside it, such as filtering or checking an item,
// keep it open, and the trigger handles its own toggle.
function onDocumentClick(event: MouseEvent): void {
  if (!open.value) return
  if (root.value && !root.value.contains(event.target as Node)) open.value = false
}

onMounted(() => document.addEventListener('click', onDocumentClick, true))
onUnmounted(() => document.removeEventListener('click', onDocumentClick, true))
</script>

<!-- The default slot renders between the trigger and the list, inside the picker, so controls
     there, such as removable chips, do not close it. -->
<template>
  <div ref="root" class="picker">
    <button
      type="button"
      class="picker-trigger"
      :aria-expanded="open"
      :disabled="disabled"
      @click="toggleOpen"
    >
      <span class="picker-summary">{{ summary }}</span>
      <svg
        class="picker-chev"
        :class="{ open }"
        width="12"
        height="12"
        viewBox="0 0 12 12"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M3 4.5l3 3 3-3" />
      </svg>
    </button>

    <slot />

    <div v-if="open" class="picker-popover">
      <input
        v-model="filter"
        type="text"
        class="picker-filter"
        :placeholder="filterPlaceholder"
        spellcheck="false"
        autocomplete="off"
        autofocus
      />
      <div class="picker-list">
        <template v-for="(group, g) in matching" :key="g">
          <span v-if="group.label" class="picker-group">{{ group.label }}</span>
          <p v-if="group.total === 0" class="picker-empty">None.</p>
          <p v-else-if="group.items.length === 0" class="picker-empty">Nothing matches.</p>
          <AppCheckbox
            v-for="(item, i) in group.items"
            :key="i"
            :checked="modelValue.includes(item.value)"
            @click="toggle(item.value)"
          >
            {{ item.label }}
          </AppCheckbox>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* --input-bg lets a container that is already lifted, such as a form card, keep its fields one
   step darker than itself, the same as its plain inputs. */
.picker {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.picker-trigger {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--input-bg, var(--color-surface-0));
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
}

.picker-trigger:focus-visible {
  outline: none;
  border-color: var(--color-amber);
}

.picker-trigger:disabled {
  opacity: 0.5;
  cursor: default;
}

.picker-summary {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.picker-chev {
  flex-shrink: 0;
  color: var(--color-fg-muted);
  transition: transform var(--duration-fast) var(--ease-out);
}

.picker-chev.open {
  transform: rotate(180deg);
}

.picker-popover {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2);
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
}

.picker-filter {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--input-bg, var(--color-surface-0));
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
}

.picker-filter:focus {
  outline: none;
  border-color: var(--color-amber);
}

.picker-list {
  display: flex;
  flex-direction: column;
  max-height: 220px;
  overflow-y: auto;
}

.picker-group {
  padding-top: var(--space-2);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
}

.picker-group:first-child {
  padding-top: 0;
}

.picker-empty {
  margin: 0;
  padding: var(--space-1) 0;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}
</style>

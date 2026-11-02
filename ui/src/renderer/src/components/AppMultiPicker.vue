<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import AppCheckbox from './AppCheckbox.vue'
import AppInput from './AppInput.vue'

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
      class="lg-field-trigger"
      :aria-expanded="open"
      :disabled="disabled"
      @click="toggleOpen"
    >
      <span class="picker-summary">{{ summary }}</span>
      <svg
        class="lg-trigger-caret"
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

    <div v-if="open" class="picker-popover lg-float">
      <AppInput
        v-model="filter"
        icon="search"
        :placeholder="filterPlaceholder"
        spellcheck="false"
        autocomplete="off"
        autofocus
      />
      <div class="picker-list">
        <template v-for="(group, g) in matching" :key="g">
          <p v-if="group.label" class="lg-menu-heading">{{ group.label }}</p>
          <p v-if="group.total === 0" class="lg-menu-empty">None.</p>
          <p v-else-if="group.items.length === 0" class="lg-menu-empty">Nothing matches.</p>
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
.picker {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  min-width: 0;
}

.picker-summary {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* The popover opens in the flow of the form rather than floating, so the form grows with it. */
.picker-popover {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2);
  box-shadow: none;
}

.picker-list {
  display: flex;
  flex-direction: column;
  max-height: 220px;
  overflow-y: auto;
}

.picker-list .lg-menu-heading {
  padding-left: 0;
}

.picker-list .lg-menu-heading:first-child {
  padding-top: 0;
}

.picker-list .lg-menu-empty {
  padding: var(--space-1) 0;
  font-size: var(--text-xs);
}
</style>

<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import AppInput from './AppInput.vue'
import type { AppInputHandle } from './appInput'

const props = withDefaults(
  defineProps<{
    // search is the text the filter box matches, the label when absent. disabled is the reason
    // the item cannot be picked, shown beside its label; hint is muted text in the same place for
    // an item that can be picked. group puts the item under a heading; items of one group must be
    // next to each other.
    items: {
      id: string
      label: string
      title?: string
      search?: string
      disabled?: string
      hint?: string
      group?: string
    }[]
    activeId: string | null
    placeholder: string
    // Overrides what the closed trigger shows, such as an unsaved name; the list is unaffected.
    label?: string
    // Without it the dropdown has no add row.
    addLabel?: string
    emptyText: string
    // A framed trigger sits among form fields and reads as an input; the plain one sits in a header.
    framed?: boolean
    // sm is for dense rows, such as a rule list.
    size?: 'sm' | 'md'
    disabled?: boolean
    // Adds a filter box at the top of the dropdown.
    searchable?: boolean
    searchPlaceholder?: string
    // Opens the dropdown upward, for a trigger near the bottom of the window.
    above?: boolean
  }>(),
  {
    label: undefined,
    addLabel: undefined,
    framed: false,
    size: 'md',
    disabled: false,
    searchable: false,
    searchPlaceholder: 'Search',
    above: false
  }
)

const emit = defineEmits<{
  select: [id: string]
  add: []
}>()

const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)
const query = ref('')
const searchRef = ref<AppInputHandle | null>(null)

const shownItems = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.items
  return props.items.filter((item) => (item.search ?? item.label).toLowerCase().includes(q))
})

const activeLabel = computed(
  () => props.label ?? props.items.find((i) => i.id === props.activeId)?.label ?? props.placeholder
)

function headingAt(index: number): string | undefined {
  const group = shownItems.value[index].group
  if (!group) return undefined
  return index === 0 || shownItems.value[index - 1].group !== group ? group : undefined
}

async function toggleOpen(): Promise<void> {
  if (props.disabled) return
  open.value = !open.value
  if (!open.value || !props.searchable) return
  query.value = ''
  await nextTick()
  searchRef.value?.focus()
}

function pickFirstShown(): void {
  const first = shownItems.value.find((item) => !item.disabled)
  if (first) pick(first.id)
}

function pick(id: string): void {
  emit('select', id)
  open.value = false
}

function pickAdd(): void {
  emit('add')
  open.value = false
}

function onDocumentClickOutside(event: MouseEvent): void {
  if (!open.value) return
  if (rootRef.value && !rootRef.value.contains(event.target as Node)) {
    open.value = false
  }
}

onMounted(() => document.addEventListener('click', onDocumentClickOutside, true))
onBeforeUnmount(() => document.removeEventListener('click', onDocumentClickOutside, true))
</script>

<template>
  <div ref="rootRef" class="switcher">
    <button
      type="button"
      :class="
        framed ? ['lg-field-trigger', { 'lg-field-trigger--sm': size === 'sm' }] : 'plain-trigger'
      "
      :aria-expanded="open"
      :disabled="disabled"
      @click="toggleOpen"
    >
      <span class="trigger-label">{{ activeLabel }}</span>
      <svg
        class="lg-trigger-caret"
        width="9"
        height="9"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.4"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <path d="M2 3.5L5 6.5L8 3.5" />
      </svg>
    </button>

    <div
      v-if="open"
      class="dropdown lg-float"
      :class="{ above, fit: framed, 'lg-menu--sm': size === 'sm' }"
    >
      <AppInput
        v-if="searchable"
        ref="searchRef"
        v-model="query"
        class="search"
        :size="size"
        icon="search"
        spellcheck="false"
        :placeholder="searchPlaceholder"
        @keydown.enter.prevent="pickFirstShown"
        @keydown.escape="open = false"
      />
      <div class="item-list">
        <template v-for="(item, i) in shownItems" :key="item.id">
          <p v-if="headingAt(i)" class="lg-menu-heading">{{ headingAt(i) }}</p>
          <button
            type="button"
            class="lg-menu-row"
            :class="{ 'is-active': item.id === activeId }"
            :title="item.disabled ?? item.title"
            :disabled="!!item.disabled"
            @click="pick(item.id)"
          >
            <span class="lg-menu-label">{{ item.label }}</span>
            <span v-if="item.disabled" class="lg-menu-hint">{{ item.disabled }}</span>
            <span v-else-if="item.hint" class="lg-menu-hint">{{ item.hint }}</span>
          </button>
        </template>
        <p v-if="items.length === 0" class="lg-menu-empty">{{ emptyText }}</p>
        <p v-else-if="shownItems.length === 0" class="lg-menu-empty">No match.</p>
      </div>
      <button v-if="addLabel" type="button" class="add-item" @click="pickAdd">
        <span class="icon">+</span>
        <span>{{ addLabel }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.switcher {
  position: relative;
}

.plain-trigger {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.plain-trigger:hover:not(:disabled),
.plain-trigger[aria-expanded='true'] {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.plain-trigger .lg-trigger-caret {
  margin-left: 0;
}

.trigger-label {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lg-field-trigger .trigger-label {
  flex: 1;
  max-width: none;
}

.dropdown {
  position: absolute;
  top: calc(100% + var(--space-2));
  left: 0;
  z-index: 80;
  width: 260px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Under a field the dropdown is at least as wide as its trigger and otherwise fits its items, so
   a narrow trigger at the right edge of a row does not push the dropdown out of the container. */
.dropdown.fit {
  width: max-content;
  min-width: 100%;
  max-width: 320px;
}

.dropdown.above {
  top: auto;
  bottom: calc(100% + var(--space-2));
}

.search {
  margin: var(--space-2) var(--space-2) 0;
}

.item-list {
  max-height: 320px;
  overflow-y: auto;
  padding: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.add-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: none;
  border-top: 1px solid var(--color-border-default);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.add-item:hover {
  background: var(--color-amber);
  color: var(--color-black);
}

.icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  font-size: var(--text-sm);
  line-height: 1;
}
</style>

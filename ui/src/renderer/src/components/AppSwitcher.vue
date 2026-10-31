<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    // search is the text the filter box matches, the label when absent. disabled is the reason
    // the item cannot be picked, shown beside its label; hint is muted text in the same place for
    // an item that can be picked.
    items: {
      id: string
      label: string
      title?: string
      search?: string
      disabled?: string
      hint?: string
    }[]
    activeId: string | null
    placeholder: string
    // Overrides what the closed trigger shows, such as an unsaved name; the list is unaffected.
    label?: string
    // Without it the dropdown has no add row.
    addLabel?: string
    emptyText: string
    // A framed trigger sits among form fields; the plain one sits in a header.
    framed?: boolean
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
const searchRef = ref<HTMLInputElement | null>(null)

const shownItems = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.items
  return props.items.filter((item) => (item.search ?? item.label).toLowerCase().includes(q))
})

const activeLabel = computed(
  () => props.label ?? props.items.find((i) => i.id === props.activeId)?.label ?? props.placeholder
)

async function toggleOpen(): Promise<void> {
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
      class="trigger"
      :class="{ framed }"
      :aria-expanded="open"
      @click="toggleOpen"
    >
      <span class="trigger-label">{{ activeLabel }}</span>
      <svg
        class="caret"
        :class="{ open }"
        width="9"
        height="9"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.4"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M2 3.5L5 6.5L8 3.5" />
      </svg>
    </button>

    <div v-if="open" class="dropdown" :class="{ above }">
      <input
        v-if="searchable"
        ref="searchRef"
        v-model="query"
        class="search"
        type="text"
        spellcheck="false"
        :placeholder="searchPlaceholder"
        @keydown.enter.prevent="pickFirstShown"
        @keydown.escape="open = false"
      />
      <div class="item-list">
        <button
          v-for="item in shownItems"
          :key="item.id"
          type="button"
          class="item-row"
          :class="{ active: item.id === activeId }"
          :title="item.disabled ?? item.title"
          :disabled="!!item.disabled"
          @click="pick(item.id)"
        >
          <span class="item-label">{{ item.label }}</span>
          <span v-if="item.disabled" class="item-reason">{{ item.disabled }}</span>
          <span v-else-if="item.hint" class="item-reason hint">{{ item.hint }}</span>
        </button>
        <p v-if="items.length === 0" class="empty">{{ emptyText }}</p>
        <p v-else-if="shownItems.length === 0" class="empty">No match.</p>
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

.trigger {
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

.trigger:hover {
  background: var(--color-surface-2);
}

.trigger.framed {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--input-bg, var(--color-surface-0));
  border: 1px solid var(--color-border-default);
  color: var(--color-fg-primary);
}

.trigger.framed:focus-visible {
  outline: none;
  border-color: var(--color-amber);
}

.trigger-label {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.trigger.framed .trigger-label {
  flex: 1;
  max-width: none;
  text-align: left;
}

.caret {
  flex-shrink: 0;
  transition: transform var(--duration-fast) var(--ease-out);
}

.caret.open {
  transform: rotate(180deg);
}

.dropdown {
  position: absolute;
  top: calc(100% + var(--space-2));
  left: 0;
  z-index: 80;
  width: 260px;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-xl);
  overflow: hidden;
}

.dropdown.above {
  top: auto;
  bottom: calc(100% + var(--space-2));
}

.search {
  margin: var(--space-2) var(--space-2) 0;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-0);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
}

.search:focus {
  outline: none;
  border-color: var(--color-amber);
}

.item-list {
  max-height: 320px;
  overflow-y: auto;
  padding: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

/* overflow: hidden lets a flex item shrink below its content, so without this a long list
   squeezes its rows to fit max-height instead of scrolling. */
.item-row {
  flex-shrink: 0;
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  text-align: left;
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: background var(--duration-fast) var(--ease-out);
}

.item-row:hover:not(:disabled) {
  background: var(--color-surface-2);
}

.item-row:disabled {
  color: var(--color-fg-muted);
  cursor: default;
}

.item-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.item-reason {
  margin-left: auto;
  flex-shrink: 0;
  font-size: var(--text-xs);
}

.item-reason.hint {
  color: var(--color-fg-muted);
}

.item-row.active {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
  font-weight: var(--weight-medium);
}

.empty {
  padding: var(--space-2) var(--space-3);
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
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

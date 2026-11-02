<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts" generic="T extends string | number">
import { computed, ref, useAttrs, type StyleValue } from 'vue'

// The one single-line text input. The root is a wrapper so a leading icon can sit inside the
// frame: a caller's class and style land on the wrapper and set layout (flex, width), while every
// other attribute and listener (type, placeholder, autofocus, @keydown) lands on the input.
defineOptions({ inheritAttrs: false })

withDefaults(
  defineProps<{
    size?: 'sm' | 'md'
    mono?: boolean
    icon?: 'search'
  }>(),
  { size: 'md', mono: false, icon: undefined }
)

// The native input always hands back a string, so .number is applied here the way Vue applies it
// to a plain input: a value that does not parse stays a string.
const [model, modifiers] = defineModel<T>({
  required: true,
  set(value) {
    if (modifiers.trim && typeof value === 'string') return value.trim() as T
    if (modifiers.number && value !== '') {
      const n = Number(value)
      return (Number.isNaN(n) ? value : n) as T
    }
    return value
  }
})

const attrs = useAttrs()
const inputAttrs = computed(() => {
  const rest = { ...attrs }
  delete rest.class
  delete rest.style
  return rest
})

const rootStyle = computed(() => attrs.style as StyleValue)

const element = ref<HTMLInputElement | null>(null)

defineExpose({
  focus: () => element.value?.focus(),
  select: () => element.value?.select(),
  el: element
})
</script>

<template>
  <span
    class="app-input"
    :class="[`app-input--${size}`, { mono, 'has-icon': icon }, attrs.class]"
    :style="rootStyle"
  >
    <svg
      v-if="icon === 'search'"
      class="app-input-icon"
      width="13"
      height="13"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      aria-hidden="true"
    >
      <circle cx="7" cy="7" r="4.5" />
      <path d="M10.5 10.5L14 14" />
    </svg>
    <input ref="element" v-model="model" class="app-input-field" v-bind="inputAttrs" />
  </span>
</template>

<style scoped>
.app-input {
  position: relative;
  display: flex;
  align-items: center;
  min-width: 0;
}

.app-input-field {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  background: var(--layer-input);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  line-height: 1.4;
  transition: border-color var(--duration-fast) var(--ease-out);
}

.app-input--sm .app-input-field {
  padding: var(--space-1) var(--space-2);
  font-size: var(--text-xs);
}

.app-input.mono .app-input-field {
  font-family: var(--font-mono);
}

.app-input.has-icon .app-input-field {
  padding-left: calc(var(--space-3) + 13px + var(--space-2));
}

.app-input--sm.has-icon .app-input-field {
  padding-left: calc(var(--space-2) + 13px + var(--space-2));
}

.app-input-icon {
  position: absolute;
  left: var(--space-3);
  color: var(--color-fg-muted);
  pointer-events: none;
}

.app-input--sm .app-input-icon {
  left: var(--space-2);
}

.app-input-field::placeholder {
  color: var(--color-fg-muted);
}

.app-input-field:hover:not(:disabled) {
  border-color: var(--color-border-strong);
}

.app-input-field:focus {
  outline: none;
  border-color: var(--color-amber);
}

.app-input-field:disabled {
  color: var(--color-fg-disabled);
  cursor: not-allowed;
}
</style>

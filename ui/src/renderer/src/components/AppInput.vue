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

// A number input hides the browser's spinner and draws its own step buttons. Stepping goes through
// the input's own stepUp and stepDown, so step, min and max hold, and the input event keeps
// v-model in sync.
const isNumber = computed(() => attrs.type === 'number')
const isDisabled = computed(() => attrs.disabled !== undefined && attrs.disabled !== false)

function step(direction: 1 | -1): void {
  const input = element.value
  if (!input || isDisabled.value) return
  if (direction === 1) input.stepUp()
  else input.stepDown()
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.focus()
}

defineExpose({
  focus: () => element.value?.focus(),
  select: () => element.value?.select(),
  el: element
})
</script>

<template>
  <span
    class="app-input"
    :class="[`app-input--${size}`, { mono, 'has-icon': icon, 'is-number': isNumber }, attrs.class]"
    :style="rootStyle"
  >
    <AppIcon v-if="icon === 'search'" class="app-input-icon" name="search" :size="13" />
    <input ref="element" v-model="model" class="app-input-field" v-bind="inputAttrs" />
    <span v-if="isNumber" class="app-input-steps">
      <button
        type="button"
        class="app-input-step"
        tabindex="-1"
        aria-label="Decrease"
        :disabled="isDisabled"
        @click="step(-1)"
      >
        <svg
          width="10"
          height="10"
          viewBox="0 0 10 10"
          fill="none"
          stroke="currentColor"
          stroke-width="1.4"
          stroke-linecap="round"
          aria-hidden="true"
        >
          <path d="M2 5H8" />
        </svg>
      </button>
      <button
        type="button"
        class="app-input-step"
        tabindex="-1"
        aria-label="Increase"
        :disabled="isDisabled"
        @click="step(1)"
      >
        <svg
          width="10"
          height="10"
          viewBox="0 0 10 10"
          fill="none"
          stroke="currentColor"
          stroke-width="1.4"
          stroke-linecap="round"
          aria-hidden="true"
        >
          <path d="M2 5H8M5 2V8" />
        </svg>
      </button>
    </span>
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

.app-input.is-number .app-input-field {
  padding-right: calc(var(--space-2) + 44px);
  appearance: textfield;
}

.app-input.is-number .app-input-field::-webkit-inner-spin-button,
.app-input.is-number .app-input-field::-webkit-outer-spin-button {
  appearance: none;
  margin: 0;
}

.app-input-steps {
  position: absolute;
  right: var(--space-1);
  display: flex;
  gap: 2px;
}

.app-input-step {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.app-input-step:hover:not(:disabled) {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.app-input-step:disabled {
  color: var(--color-fg-disabled);
  cursor: not-allowed;
}

.app-input--sm .app-input-step {
  width: 16px;
  height: 16px;
}

.app-input--sm.is-number .app-input-field {
  padding-right: calc(var(--space-1) + 36px);
}
</style>

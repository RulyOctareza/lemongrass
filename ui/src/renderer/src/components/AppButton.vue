<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { ref } from 'vue'

withDefaults(
  defineProps<{
    variant?: 'primary' | 'ghost' | 'danger'
  }>(),
  { variant: 'ghost' }
)

const el = ref<HTMLButtonElement | null>(null)

defineExpose({ focus: () => el.value?.focus() })
</script>

<!-- Attributes such as type, disabled and click handlers fall through to the native button, and
     with no type set it keeps the browser default, so a button inside a form still submits it. -->
<template>
  <button ref="el" class="app-button" :class="variant">
    <slot />
  </button>
</template>

<style scoped>
.app-button {
  padding: var(--space-1) var(--space-4);
  border-radius: var(--radius-pill);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  white-space: nowrap;
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.app-button:focus-visible {
  outline: 2px solid var(--color-fg-accent);
  outline-offset: 2px;
}

.app-button:disabled {
  opacity: 0.5;
  cursor: default;
}

.primary {
  background: var(--color-amber);
  border: none;
  color: var(--color-black);
}

.primary:hover:not(:disabled) {
  background: var(--color-amber-dim);
}

/* A faded amber reads as olive next to live amber, so a disabled primary is neutral instead. */
.primary:disabled {
  opacity: 1;
  background: var(--color-surface-3);
  color: var(--color-fg-muted);
}

.ghost {
  background: transparent;
  border: 1px solid var(--color-border-default);
  color: var(--color-fg-secondary);
}

.ghost:hover:not(:disabled) {
  color: var(--color-fg-primary);
  background: var(--color-surface-2);
}

.danger {
  background: transparent;
  border: 1px solid var(--color-error);
  color: var(--color-error);
}

.danger:hover:not(:disabled) {
  background: var(--color-error-muted);
}
</style>

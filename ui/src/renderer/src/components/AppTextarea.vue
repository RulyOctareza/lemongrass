<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { ref } from 'vue'

// The textarea is the root, so id, rows, placeholder, disabled and listeners fall through, and a
// caller's class can still set layout such as flex or min-height.
withDefaults(
  defineProps<{
    size?: 'sm' | 'md'
    resize?: 'vertical' | 'none'
    mono?: boolean
  }>(),
  { size: 'md', resize: 'vertical', mono: false }
)

const model = defineModel<string>({ default: '' })
const element = ref<HTMLTextAreaElement | null>(null)

defineExpose({ focus: () => element.value?.focus() })
</script>

<template>
  <textarea
    ref="element"
    v-model="model"
    class="app-textarea"
    :class="[`app-textarea--${size}`, { 'app-textarea--fixed': resize === 'none', mono }]"
  />
</template>

<style scoped>
.app-textarea {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--layer-input);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  line-height: 1.55;
  resize: vertical;
}

.app-textarea--sm {
  font-size: var(--text-xs);
}

.app-textarea--md {
  font-size: var(--text-sm);
}

.app-textarea--fixed {
  resize: none;
}

.app-textarea.mono {
  font-family: var(--font-mono);
}

.app-textarea::placeholder {
  color: var(--color-fg-muted);
}

.app-textarea:hover {
  border-color: var(--color-border-strong);
}

.app-textarea:focus {
  outline: none;
  border-color: var(--color-amber);
}

.app-textarea:disabled {
  color: var(--color-fg-disabled);
  cursor: not-allowed;
}
</style>

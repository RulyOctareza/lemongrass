<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'

withDefaults(
  defineProps<{
    title?: string
    size?: 'sm' | 'md' | 'lg'
    scope?: 'app' | 'container'
    flush?: boolean
    label?: string
  }>(),
  { title: undefined, size: 'sm', scope: 'container', flush: false, label: undefined }
)

const emit = defineEmits<{
  close: []
}>()

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') emit('close')
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="modal-overlay" :class="scope" @click.self="emit('close')">
    <div
      class="modal-dialog"
      :class="[size, { flush }]"
      role="dialog"
      aria-modal="true"
      :aria-label="label ?? title"
    >
      <button class="modal-close" aria-label="Close" @click="emit('close')">
        <svg
          width="12"
          height="12"
          viewBox="0 0 12 12"
          fill="none"
          stroke="currentColor"
          stroke-width="1.3"
          stroke-linecap="round"
        >
          <path d="M2.5 2.5l7 7M9.5 2.5l-7 7" />
        </svg>
      </button>
      <h3 v-if="title" class="modal-title">{{ title }}</h3>
      <div class="modal-body">
        <slot />
      </div>
      <div v-if="$slots.footer" class="modal-footer">
        <slot name="footer" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  z-index: 60;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  background: var(--color-bg-overlay);
}

.modal-overlay.container {
  position: absolute;
  inset: 0;
}

.modal-overlay.app {
  position: fixed;
  inset: 0;
  z-index: 900;
}

.modal-dialog {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  width: 100%;
  min-height: 0;
  max-height: 100%;
  padding: var(--space-6);
  overflow: hidden;
  background: var(--color-surface-1);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-xl);
}

.modal-dialog.sm {
  max-width: 420px;
}

.modal-dialog.md {
  max-width: 640px;
}

.modal-dialog.lg {
  max-width: 1000px;
  height: min(680px, 100%);
}

.modal-dialog.flush {
  gap: 0;
  padding: 0;
}

.modal-close {
  position: absolute;
  top: var(--space-3);
  right: var(--space-3);
  z-index: 2;
  display: flex;
  padding: var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.modal-close:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.modal-title {
  padding-right: var(--space-6);
  overflow-wrap: anywhere;
  color: var(--color-fg-primary);
  font-family: var(--font-display);
  font-size: var(--text-md);
  font-weight: var(--weight-semibold);
}

.modal-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.modal-dialog.lg .modal-body {
  flex: 1;
}

.modal-dialog.flush .modal-body {
  flex: 1;
  gap: 0;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
}
</style>

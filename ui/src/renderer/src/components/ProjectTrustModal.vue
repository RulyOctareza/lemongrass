<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import AppModal from './AppModal.vue'
import type { Project } from '../../../preload/types'

defineProps<{
  project: Project
  busy: boolean
}>()

const emit = defineEmits<{
  answer: [trusted: boolean]
  close: []
}>()

const declineButton = ref<HTMLButtonElement | null>(null)

onMounted(() => void nextTick(() => declineButton.value?.focus()))
</script>

<template>
  <AppModal scope="app" title="Do you trust this project?" @close="emit('close')">
    <p class="trust-path">{{ project.path }}</p>
    <p class="trust-text">
      Trusting it lets Claude Code on every account and Codex open this folder without asking again.
      They may run its hooks, settings and tools. Trust only folders whose files you know.
    </p>
    <template #footer>
      <button
        ref="declineButton"
        class="ghost-button"
        :disabled="busy"
        @click="emit('answer', false)"
      >
        Don't trust
      </button>
      <button class="primary-button" :disabled="busy" @click="emit('answer', true)">Trust</button>
    </template>
  </AppModal>
</template>

<style scoped>
.trust-path {
  margin: 0 0 var(--space-3);
  color: var(--color-fg-primary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  word-break: break-all;
}

.trust-text {
  margin: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-normal);
}

.ghost-button,
.primary-button {
  padding: var(--space-1) var(--space-4);
  border-radius: var(--radius-pill);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.ghost-button {
  background: transparent;
  border: 1px solid var(--color-border-default);
  color: var(--color-fg-secondary);
}

.ghost-button:hover {
  color: var(--color-fg-primary);
  background: var(--color-surface-2);
}

.primary-button {
  background: var(--color-amber);
  border: none;
  color: var(--color-black);
}

.primary-button:hover {
  background: var(--color-amber-dim);
}

.ghost-button:disabled,
.primary-button:disabled {
  opacity: 0.5;
  cursor: default;
}
</style>

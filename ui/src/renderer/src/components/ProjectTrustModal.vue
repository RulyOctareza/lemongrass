<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import AppModal from './AppModal.vue'
import type { Project } from '../../../preload/types'
import AppButton from './AppButton.vue'

defineProps<{
  project: Project
  busy: boolean
}>()

const emit = defineEmits<{
  answer: [trusted: boolean]
  close: []
}>()

const declineButton = ref<InstanceType<typeof AppButton> | null>(null)

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
      <AppButton ref="declineButton" :disabled="busy" @click="emit('answer', false)">
        Don't trust
      </AppButton>
      <AppButton variant="primary" :disabled="busy" @click="emit('answer', true)">Trust</AppButton>
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
</style>

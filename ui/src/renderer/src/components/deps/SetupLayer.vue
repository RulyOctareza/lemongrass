<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import AppModal from '../AppModal.vue'
import AppButton from '../AppButton.vue'
import MarkedText from '../MarkedText.vue'
import DepRow from './DepRow.vue'
import { depsStore } from '../../deps'
import type { NamePart } from '../../tabNames'

// A check that finishes sooner than this never shows the spinner, so a cached check does not flash it.
const SPINNER_DELAY_MS = 300

// Closing without Continue hides the layer for this run only; setupVersion stays unsaved, so it
// shows again on the next startup.
const hidden = ref(false)

const state = computed(() => depsStore.state)
const open = computed(() => !hidden.value && !!state.value?.setupDue)
const required = computed(() => state.value?.report?.deps.filter((dep) => dep.required) ?? [])
const pending = computed(() => required.value.filter((dep) => dep.status !== 'ok'))
const waiting = computed(() => state.value?.phase === 'checking' && !state.value.report)

const spinnerShown = ref(false)
let spinnerTimer: ReturnType<typeof setTimeout> | undefined
watch(
  waiting,
  (now) => {
    clearTimeout(spinnerTimer)
    spinnerShown.value = false
    if (now) spinnerTimer = setTimeout(() => (spinnerShown.value = true), SPINNER_DELAY_MS)
  },
  { immediate: true }
)
onBeforeUnmount(() => clearTimeout(spinnerTimer))

const manageParts: NamePart[] = [
  { text: 'You can manage other dependencies in ' },
  { text: 'Settings', mark: true },
  { text: ', under ' },
  { text: 'Dependencies', mark: true },
  { text: '.' }
]

// The install button carries the weight while a required dependency is pending, so Continue steps down.
const continueVariant = computed(() =>
  pending.value.some((dep) => dep.offer) ? 'ghost' : 'primary'
)

async function finish(): Promise<void> {
  hidden.value = true
  await window.api.deps.setupDone()
}
</script>

<template>
  <AppModal
    v-if="open && state"
    scope="app"
    size="md"
    title="Dependency Check"
    @close="hidden = true"
  >
    <div class="setup">
      <template v-if="waiting">
        <div v-if="spinnerShown" class="setup-checking">
          <span class="setup-spinner" />
          <p class="settings-text">Optimizing Lemongrass</p>
        </div>
      </template>

      <template v-else-if="state.phase === 'failed' && !state.report">
        <p class="settings-error">{{ state.message }}</p>
        <p class="settings-text">You can check again from Settings, under Dependencies.</p>
      </template>

      <template v-else-if="state.report">
        <p v-if="pending.length === 0" class="settings-text">
          Everything lemongrass needs is in place.
        </p>
        <p v-else class="settings-text">
          Lemongrass needs these tools. Without them, the features they serve stay off.
        </p>
        <ul class="settings-rows">
          <DepRow v-for="dep in required" :key="dep.name" :dep="dep" :state="state" />
        </ul>
        <p v-if="state.unsupported" class="settings-text">{{ state.unsupported }}</p>
        <p class="settings-text"><MarkedText :parts="manageParts" /></p>
      </template>
    </div>

    <template #footer>
      <AppButton type="button" :variant="continueVariant" @click="finish">Continue</AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.setup {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.setup-checking {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.setup-spinner {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  border: 2px solid var(--color-border-default);
  border-top-color: var(--color-amber);
  border-radius: 50%;
  animation: setup-spin 0.8s linear infinite;
}

@keyframes setup-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>

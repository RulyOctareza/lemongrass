<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppModal from '../AppModal.vue'
import AppButton from '../AppButton.vue'
import DepRow from './DepRow.vue'
import { depsStore } from '../../deps'
import { listText } from '../../depsView'

// Closing without Continue hides the layer for this run only; setupVersion stays unsaved, so it
// shows again on the next startup.
const hidden = ref(false)

const state = computed(() => depsStore.state)
const open = computed(() => !hidden.value && !!state.value?.setupDue)
const required = computed(() => state.value?.report?.deps.filter((dep) => dep.required) ?? [])
const pending = computed(() => required.value.filter((dep) => dep.status !== 'ok'))
const optionalMissing = computed(
  () =>
    state.value?.report?.deps
      .filter((dep) => !dep.required && dep.status !== 'ok')
      .map((dep) => dep.name) ?? []
)

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
    title="Optimizing lemongrass"
    @close="hidden = true"
  >
    <div class="setup">
      <div v-if="state.phase === 'checking' && !state.report" class="setup-checking">
        <span class="setup-spinner" />
        <p class="settings-text">Checking the tools lemongrass uses from your system.</p>
      </div>

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
        <p v-if="optionalMissing.length" class="settings-text">
          {{ listText(optionalMissing) }} {{ optionalMissing.length === 1 ? 'is' : 'are' }}
          optional. Settings, under Dependencies, installs
          {{ optionalMissing.length === 1 ? 'it' : 'them' }}.
        </p>
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

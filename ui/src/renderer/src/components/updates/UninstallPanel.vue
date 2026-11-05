<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AppButton from '../AppButton.vue'
import AppModal from '../AppModal.vue'
import AppProgressLine from '../AppProgressLine.vue'
import type { UninstallState } from '../../../../preload/types'
import { isWindows } from '../../platform'

const state = ref<UninstallState | null>(null)
const confirming = ref(false)
let unsubscribe: (() => void) | null = null

onMounted(async () => {
  unsubscribe = window.api.update.onUninstallState((next) => (state.value = next))
  state.value = await window.api.update.uninstallState()
})

onBeforeUnmount(() => unsubscribe?.())

function start(): void {
  confirming.value = false
  void window.api.update.uninstall()
}

const running = computed(() =>
  ['cleaning', 'authorizing', 'removing'].includes(state.value?.phase ?? '')
)

const status = computed((): { text: string; tone: '' | 'error' } => {
  switch (state.value?.phase) {
    case 'cleaning':
      return { text: 'Removing the keeper, hooks, skills and binaries.', tone: '' }
    case 'authorizing':
      return { text: 'Waiting for your password.', tone: '' }
    case 'removing':
      // Windows hands the removal to the uninstaller, which runs silently once the app has closed.
      return isWindows
        ? { text: 'Lemongrass closes now. Its uninstaller finishes the removal.', tone: '' }
        : { text: 'Removing the package. Lemongrass closes when it is done.', tone: '' }
    case 'failed':
      return { text: state.value.message ?? 'Uninstalling failed.', tone: 'error' }
    default:
      return { text: '', tone: '' }
  }
})
</script>

<template>
  <section v-if="state && state.phase !== 'unavailable'" class="uninstall">
    <h3 class="settings-subtitle">Uninstall</h3>
    <p class="settings-text">
      Removes Lemongrass from this computer and closes it. Your Lemongrass data stays.
    </p>
    <AppProgressLine
      v-if="state.phase === 'cleaning' || state.phase === 'removing'"
      label="Uninstalling Lemongrass"
    />
    <pre v-if="state.phase === 'failed' && state.output" class="uninstall-output">{{
      state.output
    }}</pre>
    <div class="settings-footer">
      <span class="settings-footer-status" :class="status.tone">{{ status.text }}</span>
      <AppButton type="button" :disabled="running" @click="confirming = true">
        Uninstall Lemongrass
      </AppButton>
    </div>

    <AppModal v-if="confirming" title="Uninstall Lemongrass" @close="confirming = false">
      <p class="settings-text">This removes:</p>
      <ul class="uninstall-list">
        <li>The background keeper service.</li>
        <li>The Lemongrass hooks and status line in your Claude and Codex settings.</li>
        <li>The Lemongrass skills and the lgrass, lgrassconf and lgrassd commands.</li>
        <li v-if="isWindows">
          The Lemongrass app, removed by its uninstaller after Lemongrass closes.
        </li>
        <li v-else>The Lemongrass package. This asks for your system password.</li>
      </ul>
      <p class="settings-text">
        Your Lemongrass data, Claude account folders and trusted projects stay. Lemongrass closes
        when it is done, and every open tab closes with it.
      </p>
      <template #footer>
        <AppButton @click="confirming = false">Cancel</AppButton>
        <AppButton variant="danger" @click="start">Uninstall</AppButton>
      </template>
    </AppModal>
  </section>
</template>

<style scoped>
.uninstall {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.uninstall-list {
  margin: 0;
  padding-left: var(--space-5);
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.uninstall-output {
  margin: 0;
  max-height: 160px;
  overflow: auto;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border-radius: var(--radius-md, 8px);
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  white-space: pre-wrap;
}
</style>

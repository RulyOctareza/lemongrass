<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AppButton from '../AppButton.vue'
import AppConfirmButton from '../AppConfirmButton.vue'
import AppProgressLine from '../AppProgressLine.vue'
import UninstallPanel from './UninstallPanel.vue'
import type { UpdateState } from '../../../../preload/types'

const state = ref<UpdateState | null>(null)
let unsubscribe: (() => void) | null = null

onMounted(async () => {
  unsubscribe = window.api.update.onState((next) => (state.value = next))
  state.value = await window.api.update.state()
})

onBeforeUnmount(() => unsubscribe?.())

const check = (): Promise<UpdateState> => window.api.update.check()
const install = (): Promise<void> => window.api.update.install()
const restart = (): Promise<void> => window.api.update.restart()

const working = computed(() =>
  ['checking', 'downloading', 'verifying', 'authorizing', 'installing'].includes(
    state.value?.phase ?? ''
  )
)

// The download fills the line by its fraction; verifying and installing have no measure and sweep.
// The password wait shows none, since nothing moves until the prompt is answered.
const line = computed((): { label: string; value: number | null } | null => {
  switch (state.value?.phase) {
    case 'downloading':
      return { label: 'Downloading the update', value: state.value.progress ?? 0 }
    case 'verifying':
      return { label: 'Checking the download', value: null }
    case 'installing':
      return { label: 'Installing the update', value: null }
    default:
      return null
  }
})

const canCheck = computed(
  () => !!state.value && !working.value && !['unsupported', 'installed'].includes(state.value.phase)
)

const canInstall = computed(
  () => !!state.value?.available && ['idle', 'failed'].includes(state.value.phase)
)

const checkedAt = computed(() =>
  state.value?.checkedAt ? new Date(state.value.checkedAt).toLocaleString() : 'Not yet'
)

const status = computed((): { text: string; tone: '' | 'muted' | 'saved' | 'error' } => {
  const s = state.value
  if (!s) return { text: '', tone: 'muted' }
  switch (s.phase) {
    case 'unsupported':
      return { text: s.message ?? '', tone: 'muted' }
    case 'checking':
      return { text: 'Checking for updates.', tone: 'muted' }
    case 'downloading':
      return { text: `Downloading ${s.latest}. ${Math.round((s.progress ?? 0) * 100)}%`, tone: '' }
    case 'verifying':
      return { text: 'Checking the download.', tone: '' }
    case 'authorizing':
      return { text: 'Waiting for your password.', tone: '' }
    // The platform's own message wins: on macOS installing only prepares the new app, and the
    // password, when one is needed, is asked on restart.
    case 'installing':
      return { text: s.message ?? `Installing ${s.latest}.`, tone: '' }
    case 'installed':
      return {
        text:
          s.message ??
          `Version ${s.latest} is installed. Restart to use it. Restarting closes every open tab.`,
        tone: 'saved'
      }
    case 'failed':
      return { text: s.message ?? 'The update failed.', tone: 'error' }
    default:
      if (s.message) return { text: s.message, tone: 'muted' }
      if (s.available) return { text: `Version ${s.latest} is ready to install.`, tone: '' }
      if (s.latest) return { text: 'You have the latest version.', tone: 'muted' }
      return { text: 'Not checked yet.', tone: 'muted' }
  }
})
</script>

<template>
  <div v-if="state" class="updates">
    <p class="settings-text">
      Lemongrass looks for a new release every few hours. Installing one can ask for your system
      password.
    </p>

    <dl class="settings-facts">
      <dt>Installed</dt>
      <dd>{{ state.current }}</dd>
      <dt>Latest release</dt>
      <dd>{{ state.latest ?? 'Unknown' }}</dd>
      <dt>Last checked</dt>
      <dd>{{ checkedAt }}</dd>
    </dl>

    <AppProgressLine v-if="line" :label="line.label" :value="line.value" />

    <div class="settings-footer">
      <span class="settings-footer-status" :class="status.tone">{{ status.text }}</span>
      <AppButton type="button" :disabled="!canCheck" @click="check"> Check now </AppButton>
      <AppButton v-if="canInstall" type="button" variant="primary" @click="install">
        Install {{ state.latest }}
      </AppButton>
      <AppConfirmButton
        v-if="state.phase === 'installed'"
        v-slot="{ armed }"
        variant="primary"
        @confirm="restart"
      >
        {{ armed ? 'Click again' : 'Restart now' }}
      </AppConfirmButton>
    </div>

    <UninstallPanel />
  </div>
</template>

<style scoped>
.updates {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
</style>

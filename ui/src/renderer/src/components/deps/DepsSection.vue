<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import AppButton from '../AppButton.vue'
import DepRow from './DepRow.vue'
import { depsStore } from '../../deps'
import { osText } from '../../depsView'
import type { DepsState } from '../../../../preload/types'

const state = computed(() => depsStore.state)

const status = computed((): { text: string; tone: '' | 'muted' | 'error' } => {
  const s = state.value
  if (!s) return { text: '', tone: 'muted' }
  if (s.phase === 'checking') return { text: 'Checking your system.', tone: 'muted' }
  if (s.phase === 'failed') return { text: s.message ?? 'The check failed.', tone: 'error' }
  if (s.unsupported) return { text: s.unsupported, tone: 'muted' }
  return { text: '', tone: 'muted' }
})

const check = (): Promise<DepsState> => window.api.deps.check()
</script>

<template>
  <div v-if="state" class="deps">
    <p class="settings-text">
      The tools lemongrass uses from your system. Installs and updates go through your system's own
      package manager.
    </p>

    <dl v-if="state.report" class="settings-facts">
      <dt>System</dt>
      <dd>{{ osText(state.report.os) }}</dd>
    </dl>

    <ul v-if="state.report" class="settings-rows">
      <DepRow v-for="dep in state.report.deps" :key="dep.name" :dep="dep" :state="state" />
    </ul>

    <div class="settings-footer">
      <span class="settings-footer-status" :class="status.tone">{{ status.text }}</span>
      <AppButton
        type="button"
        :disabled="state.phase === 'checking' || !!state.installing"
        @click="check"
      >
        Check again
      </AppButton>
    </div>
  </div>
</template>

<style scoped>
.deps {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
</style>

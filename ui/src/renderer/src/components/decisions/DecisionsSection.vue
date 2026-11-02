<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The asks waiting on the human, first in the Vault nav. Clicking one opens the same modal as the
// notification. Its styles come from vault.css, so it must render inside a .vault-tab element.
import { onMounted, ref, watch } from 'vue'
import type { Ask } from '../../../../preload/types'
import { askState, openAsk } from '../../asks'
import { describeError, vaultSession } from '../../vaultSession'
import { askStateLabel, askSummary } from './askLabels'
import { askParts, askWhere, loadAskProjects } from './askOrigin'
import MarkedText from '../MarkedText.vue'
import AppEmptyState from '../AppEmptyState.vue'
import { decisionsIcon } from '../../icons'

const props = defineProps<{
  visible: boolean
}>()

const asks = ref<Ask[]>([])
const loading = ref(false)
const listError = ref('')

async function refresh(): Promise<void> {
  if (!vaultSession.unlocked) return
  loading.value = asks.value.length === 0
  try {
    asks.value = await window.api.ask.list(vaultSession.passphrase)
    listError.value = ''
  } catch (err) {
    listError.value = describeError(err)
  } finally {
    loading.value = false
  }
}

function age(iso: string): string {
  const minutes = Math.floor((Date.now() - new Date(iso).getTime()) / 60_000)
  if (!Number.isFinite(minutes) || minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes} min ago`
  return `${Math.floor(minutes / 60)} h ago`
}

onMounted(() => {
  loadAskProjects()
  void refresh()
})

watch(
  () => [props.visible, askState.version] as const,
  ([visible]) => {
    if (visible) void refresh()
  }
)
</script>

<template>
  <div class="section">
    <div class="section-toolbar">
      <p class="hint">
        Requests from agents that only you can answer. Open one to see what the agent wants and why.
      </p>
    </div>

    <p v-if="listError" class="error-text">{{ listError }}</p>
    <AppEmptyState v-else-if="loading" layout="fill" loading title="Loading..." />
    <AppEmptyState
      v-else-if="asks.length === 0"
      layout="fill"
      :icon="decisionsIcon"
      title="Nothing is waiting for your decision."
    />

    <div v-else class="channel-list">
      <button
        v-for="ask in asks"
        :key="ask.id"
        type="button"
        class="channel-row decision-row"
        @click="openAsk(ask.id)"
      >
        <span class="channel-info">
          <span class="channel-name"><MarkedText :parts="askParts(ask)" /></span>
          <span v-if="ask.type !== 'db'" class="decision-summary">{{ askSummary(ask) }}</span>
          <span class="channel-db">Asked {{ age(ask.created_at) }} in {{ askWhere(ask) }}</span>
        </span>
        <span
          class="status-capsule"
          :class="ask.state === 'running' ? 'status-verifying' : 'status-open'"
        >
          {{ askStateLabel(ask.state) }}
        </span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.decision-row {
  flex-wrap: nowrap;
  width: 100%;
  border: none;
  color: inherit;
  text-align: left;
  font: inherit;
  cursor: pointer;
}

.decision-row .channel-info {
  flex: 1;
  min-width: 0;
}

.decision-row:hover {
  background: var(--layer-card-hover);
}

.decision-summary {
  overflow: hidden;
  color: var(--color-fg-primary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

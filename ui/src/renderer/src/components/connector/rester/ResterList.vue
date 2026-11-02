<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { describeError, vaultSession } from '../../../vaultSession'
import type { VaultResterSummary } from '../../../../../preload/types'
import { matchesQuery, resterDomains } from '../listFilter'
import AppButton from '../../AppButton.vue'
import ResterCreateForm from './ResterCreateForm.vue'
import ResterDetail from './ResterDetail.vue'

const props = defineProps<{
  visible: boolean
}>()

const resters = ref<string[]>([])
const loading = ref(false)
const listError = ref('')
const showCreate = ref(false)
const openName = ref<string | null>(null)
const deletingName = ref<string | null>(null)
const query = ref('')
const summaries = ref<Record<string, VaultResterSummary>>({})
const summaryError = ref('')

const shownResters = computed(() =>
  resters.value.filter((name) =>
    matchesQuery(query.value, [name, ...resterDomains(summaries.value[name])])
  )
)

// Domains live inside the encrypted rester, so a failure here leaves search matching names only.
async function refreshSummaries(): Promise<void> {
  summaryError.value = ''
  try {
    const list = await window.api.vault.listResterSummaries(vaultSession.passphrase)
    summaries.value = Object.fromEntries(list.map((s) => [s.Name, s]))
  } catch (err) {
    summaryError.value = `Search covers names only. ${describeError(err)}`
  }
}

async function refresh(): Promise<void> {
  loading.value = true
  listError.value = ''
  try {
    resters.value = await window.api.vault.listResters()
  } catch (err) {
    listError.value = describeError(err)
  } finally {
    loading.value = false
  }
  refreshSummaries()
}

async function deleteRester(name: string): Promise<void> {
  deletingName.value = name
  try {
    await window.api.vault.deleteRester(name)
    await refresh()
  } catch (err) {
    listError.value = describeError(err)
  } finally {
    deletingName.value = null
  }
}

async function onCreated(name: string): Promise<void> {
  showCreate.value = false
  await refresh()
  openName.value = name
}

async function closeDetail(): Promise<void> {
  openName.value = null
  await refresh()
}

onMounted(refresh)

watch(
  () => props.visible,
  (visible) => {
    if (visible && openName.value === null) refresh()
  }
)
</script>

<template>
  <div class="section">
    <ResterDetail v-if="openName !== null" :name="openName" @close="closeDetail" />

    <template v-else>
      <div class="section-toolbar">
        <p class="hint">
          Saved systems with their environments, users and login. Channels are created from a
          rester.
        </p>
        <AppButton
          v-if="!showCreate"
          variant="primary"
          class="new-channel-button"
          @click="showCreate = true"
        >
          + New rester
        </AppButton>
      </div>

      <ResterCreateForm
        v-if="showCreate"
        :taken="resters"
        @created="onCreated"
        @cancel="showCreate = false"
      />

      <p v-if="listError" class="error-text">{{ listError }}</p>
      <p v-else-if="loading" class="empty">Loading...</p>
      <p v-else-if="resters.length === 0" class="empty">No resters yet.</p>

      <template v-else>
        <div class="list-filter">
          <input
            v-model="query"
            type="text"
            spellcheck="false"
            placeholder="Search by name or domain"
          />
        </div>
        <p v-if="summaryError" class="error-text">{{ summaryError }}</p>
        <p v-if="shownResters.length === 0" class="empty">No rester matches.</p>

        <div v-else class="channel-list">
          <div v-for="name in shownResters" :key="name" class="channel-row">
            <div class="channel-info">
              <span class="channel-db">{{ name }}</span>
            </div>
            <div class="channel-actions">
              <AppButton @click="openName = name">Open</AppButton>
              <AppButton
                variant="danger"
                :disabled="deletingName === name"
                @click="deleteRester(name)"
              >
                {{ deletingName === name ? 'Deleting...' : 'Delete' }}
              </AppButton>
            </div>
          </div>
        </div>
      </template>
    </template>
  </div>
</template>

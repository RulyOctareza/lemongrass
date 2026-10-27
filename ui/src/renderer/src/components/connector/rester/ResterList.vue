<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { describeError } from '../../../vaultSession'
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

      <div v-else class="channel-list">
        <div v-for="name in resters" :key="name" class="channel-row">
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
  </div>
</template>

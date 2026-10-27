<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { VaultEnvironment } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import AppModal from '../../AppModal.vue'
import { headerDrafts, headersFromDrafts, type HeaderDraft } from './resterDraft'

const props = defineProps<{
  title: string
  initial: VaultEnvironment
  // The name is locked when editing, since channels and user assignments refer to it.
  lockName: boolean
  taken: string[]
  saving: boolean
  error: string
}>()

const emit = defineEmits<{
  save: [environment: VaultEnvironment]
  close: []
}>()

const name = ref(props.initial.Name)
const baseUrl = ref(props.initial.BaseURL)
const origin = ref(props.initial.Origin)
const headers = ref<HeaderDraft[]>(headerDrafts(props.initial.Headers))

const nameTaken = computed(() => !props.lockName && props.taken.includes(name.value.trim()))
const canSave = computed(() => name.value.trim() !== '' && !nameTaken.value && !props.saving)

function save(): void {
  if (!canSave.value) return
  emit('save', {
    Name: name.value.trim(),
    BaseURL: baseUrl.value.trim(),
    Origin: origin.value.trim(),
    Headers: headersFromDrafts(headers.value)
  })
}
</script>

<template>
  <AppModal scope="app" size="md" :title="title" @close="emit('close')">
    <div class="env-form">
      <div class="field-block">
        <label class="field-label" for="env-name">Name</label>
        <input
          id="env-name"
          v-model="name"
          type="text"
          placeholder="e.g. staging"
          :disabled="lockName"
          autofocus
        />
        <p v-if="nameTaken" class="error-text">An environment with this name already exists.</p>
      </div>
      <div class="field-block">
        <label class="field-label" for="env-base-url">API base URL</label>
        <input
          id="env-base-url"
          v-model="baseUrl"
          type="text"
          placeholder="https://api.example.com"
        />
        <p class="hint">Leave blank when this environment has no API side.</p>
      </div>
      <div class="field-block">
        <label class="field-label" for="env-origin">Web origin</label>
        <input id="env-origin" v-model="origin" type="text" placeholder="https://example.com" />
        <p class="hint">Leave blank when this environment has no web side.</p>
      </div>
      <div class="field-block">
        <span class="field-label">Extra headers</span>
        <p class="hint">Sent with the shared API headers. A header here wins on the same name.</p>
        <div class="kv-list">
          <div v-for="(header, i) in headers" :key="i" class="kv-row">
            <input v-model="header.name" type="text" placeholder="name" />
            <input v-model="header.value" type="text" placeholder="value" />
            <button
              type="button"
              class="chip-remove"
              title="Remove header"
              @click="headers.splice(i, 1)"
            >
              ×
            </button>
          </div>
          <AppButton
            type="button"
            class="new-channel-button"
            @click="headers.push({ name: '', value: '' })"
          >
            + Header
          </AppButton>
        </div>
      </div>
      <p v-if="error" class="error-text">{{ error }}</p>
    </div>
    <template #footer>
      <AppButton @click="emit('close')">Cancel</AppButton>
      <AppButton variant="primary" :disabled="!canSave" @click="save">
        {{ saving ? 'Saving...' : 'Save' }}
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.env-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
  /* AppModal's body does not scroll, so a long form scrolls itself. */
  overflow-y: auto;
}
</style>

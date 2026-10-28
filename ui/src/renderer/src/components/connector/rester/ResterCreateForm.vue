<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, ref, toRef } from 'vue'
import { vaultSession, describeError } from '../../../vaultSession'
import type { VaultRester } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import AppCheckbox from '../../AppCheckbox.vue'
import { defaultResterAPI } from './resterDraft'

const props = defineProps<{
  taken: string[]
}>()

const emit = defineEmits<{
  created: [name: string]
  cancel: []
}>()

const sessionPassphrase = toRef(vaultSession, 'passphrase')

const name = ref('')
const envName = ref('')
const baseUrl = ref('')
const origin = ref('')
const withApi = ref(true)
const withWeb = ref(false)
const withApp = ref(false)
const packageName = ref('')
const saving = ref(false)
const formError = ref('')

const nameTaken = computed(() => props.taken.includes(name.value.trim()))

const canSave = computed(
  () =>
    name.value.trim() !== '' &&
    !nameTaken.value &&
    envName.value.trim() !== '' &&
    (withApi.value || withWeb.value || withApp.value) &&
    (!withApp.value || packageName.value.trim() !== '')
)

async function save(): Promise<void> {
  if (!canSave.value || saving.value) return
  saving.value = true
  formError.value = ''
  const rester: VaultRester = {
    API: withApi.value ? defaultResterAPI() : null,
    Web: withWeb.value ? { Login: null } : null,
    App: withApp.value ? { Package: packageName.value.trim(), Login: null } : null,
    Environments: [
      {
        Name: envName.value.trim(),
        BaseURL: baseUrl.value.trim(),
        Origin: origin.value.trim(),
        Headers: null
      }
    ],
    Users: []
  }
  try {
    await window.api.vault.putRester(sessionPassphrase.value, name.value.trim(), rester)
    emit('created', name.value.trim())
  } catch (err) {
    formError.value = describeError(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="inline-section">
    <h4 class="inline-section-title">New rester</h4>

    <div class="field-block">
      <label class="field-label" for="rester-name">Name</label>
      <input id="rester-name" v-model="name" type="text" placeholder="e.g. billing" autofocus />
      <p v-if="nameTaken" class="error-text">A rester with this name already exists.</p>
    </div>

    <div class="field-block">
      <label class="field-label" for="rester-env-name">First environment</label>
      <input id="rester-env-name" v-model="envName" type="text" placeholder="e.g. staging" />
    </div>

    <div class="field-grid">
      <div class="field-block">
        <label class="field-label" for="rester-base-url">API base URL</label>
        <input
          id="rester-base-url"
          v-model="baseUrl"
          type="text"
          placeholder="https://api.example.com"
        />
      </div>
      <div class="field-block">
        <label class="field-label" for="rester-origin">Web origin</label>
        <input id="rester-origin" v-model="origin" type="text" placeholder="https://example.com" />
      </div>
    </div>

    <div class="field-block">
      <span class="field-label">Sides</span>
      <div class="radio-row">
        <AppCheckbox :checked="withApi" @click="withApi = !withApi">API</AppCheckbox>
        <AppCheckbox :checked="withWeb" @click="withWeb = !withWeb">Web</AppCheckbox>
        <AppCheckbox :checked="withApp" @click="withApp = !withApp">App</AppCheckbox>
      </div>
      <p class="hint">
        An environment reaches the API side through its base URL and the web side through its
        origin. The app side logs in on a phone and needs no address. Login settings and users are
        added after creating.
      </p>
    </div>

    <div v-if="withApp" class="field-block">
      <label class="field-label" for="rester-package">App package name</label>
      <input id="rester-package" v-model="packageName" type="text" placeholder="com.example.app" />
    </div>

    <div class="create-actions">
      <p v-if="formError" class="error-text">{{ formError }}</p>
      <AppButton @click="emit('cancel')">Cancel</AppButton>
      <AppButton variant="primary" :disabled="!canSave || saving" @click="save">
        {{ saving ? 'Creating...' : 'Create' }}
      </AppButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { VaultResterAPI, VaultRester, VaultTokenPlacement } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import AppSegmented from '../../AppSegmented.vue'
import {
  defaultResterAPI,
  headerDrafts,
  headersFromDrafts,
  type CommitRester,
  type HeaderDraft
} from './resterDraft'

const props = defineProps<{
  rester: VaultRester
  commit: CommitRester
}>()

type TtlMode = 'fixed' | 'field' | 'jwt'

const ttlOptions: { value: TtlMode; label: string }[] = [
  { value: 'fixed', label: 'Fixed' },
  { value: 'field', label: 'From response field' },
  { value: 'jwt', label: 'Decode as JWT' }
]

const placementOptions: { value: VaultTokenPlacement['Kind']; label: string }[] = [
  { value: 'header', label: 'Header' },
  { value: 'cookie', label: 'Cookie' },
  { value: 'query', label: 'Query param' }
]

const open = ref(false)
const saving = ref(false)
const error = ref('')

const loginEndpoint = ref('')
const tokenPath = ref('')
const ttlMode = ref<TtlMode>('fixed')
const ttlField = ref('')
const fixedTtlMinutes = ref(60)
const placementKind = ref<VaultTokenPlacement['Kind']>('header')
const placementName = ref('Authorization')
const placementPrefix = ref('Bearer ')
const headers = ref<HeaderDraft[]>([])

function fill(api: VaultResterAPI): void {
  loginEndpoint.value = api.LoginEndpoint
  tokenPath.value = api.TokenPath
  if (api.TTLOrigin === 'jwt-exp') {
    ttlMode.value = 'jwt'
    ttlField.value = ''
  } else if (api.TTLOrigin !== '') {
    ttlMode.value = 'field'
    ttlField.value = api.TTLOrigin
  } else {
    ttlMode.value = 'fixed'
    ttlField.value = ''
  }
  fixedTtlMinutes.value = api.FixedTTLSeconds / 60
  placementKind.value = api.TokenPlacement.Kind
  placementName.value = api.TokenPlacement.Name
  placementPrefix.value = api.TokenPlacement.Prefix
  headers.value = headerDrafts(api.Headers)
}

function build(): VaultResterAPI {
  return {
    LoginEndpoint: loginEndpoint.value.trim(),
    TokenPath: tokenPath.value.trim(),
    TTLOrigin:
      ttlMode.value === 'jwt' ? 'jwt-exp' : ttlMode.value === 'field' ? ttlField.value.trim() : '',
    FixedTTLSeconds: Math.round(fixedTtlMinutes.value * 60),
    TokenPlacement: {
      Kind: placementKind.value,
      Name: placementName.value.trim(),
      Prefix: placementKind.value === 'header' ? placementPrefix.value : ''
    },
    Headers: headersFromDrafts(headers.value)
  }
}

function toggle(): void {
  if (open.value) {
    open.value = false
    return
  }
  fill(props.rester.API ?? defaultResterAPI())
  error.value = ''
  open.value = true
}

async function save(): Promise<void> {
  saving.value = true
  const api = build()
  error.value = await props.commit((r) => {
    r.API = api
  })
  saving.value = false
  if (error.value === '') open.value = false
}

async function turnOff(): Promise<void> {
  saving.value = true
  error.value = await props.commit((r) => {
    r.API = null
  })
  saving.value = false
  if (error.value === '') open.value = false
}
</script>

<template>
  <div class="inline-section">
    <button type="button" class="settings-toggle" :aria-expanded="open" @click="toggle">
      <span class="settings-chevron" :class="{ expanded: open }">▸</span>
      API side
      <span class="status-capsule" :class="{ 'status-open': rester.API }">
        {{ rester.API ? 'On' : 'Off' }}
      </span>
      <span v-if="rester.API && !open" class="settings-summary">
        {{ rester.API.LoginEndpoint || 'Tokens only, no login endpoint' }}
      </span>
    </button>

    <div v-if="open" class="settings-body">
      <p v-if="!rester.API" class="hint">Saving turns the API side on for this rester.</p>

      <div class="field-block">
        <label class="field-label" for="api-login-endpoint">Login endpoint</label>
        <input
          id="api-login-endpoint"
          v-model="loginEndpoint"
          type="text"
          placeholder="/auth/login (leave blank if every user brings a token)"
        />
      </div>

      <div class="field-block">
        <label class="field-label" for="api-token-path">Token path</label>
        <input
          id="api-token-path"
          v-model="tokenPath"
          type="text"
          placeholder="e.g. access_token or data.token"
        />
        <p class="hint">
          Where the access token lives in the login response, as a dotted JSON path.
        </p>
      </div>

      <div class="field-block">
        <span id="api-ttl" class="field-label">Token lifetime</span>
        <AppSegmented v-model="ttlMode" :options="ttlOptions" aria-labelledby="api-ttl" />
        <input
          v-if="ttlMode === 'fixed'"
          v-model.number="fixedTtlMinutes"
          type="number"
          min="1"
          title="Fixed lifetime in minutes"
        />
        <input
          v-if="ttlMode === 'field'"
          v-model="ttlField"
          type="text"
          placeholder="e.g. expires_in"
        />
      </div>

      <div class="field-block">
        <span id="api-placement" class="field-label">Token placement</span>
        <AppSegmented
          v-model="placementKind"
          :options="placementOptions"
          aria-labelledby="api-placement"
        />
        <div class="field-grid">
          <input v-model="placementName" type="text" placeholder="Name, e.g. Authorization" />
          <input
            v-if="placementKind === 'header'"
            v-model="placementPrefix"
            type="text"
            placeholder="Prefix, e.g. Bearer"
          />
        </div>
      </div>

      <div class="field-block">
        <span class="field-label">Headers</span>
        <p class="hint">
          Sent on the login and on every call in every environment. Models never see them.
        </p>
        <div class="kv-list">
          <div v-for="(header, i) in headers" :key="i" class="kv-row">
            <input v-model="header.name" type="text" placeholder="e.g. api-version" />
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

      <div class="create-actions">
        <p v-if="error" class="error-text">{{ error }}</p>
        <AppButton v-if="rester.API" variant="danger" :disabled="saving" @click="turnOff">
          Turn off
        </AppButton>
        <AppButton @click="open = false">Cancel</AppButton>
        <AppButton variant="primary" :disabled="saving" @click="save">
          {{ saving ? 'Saving...' : 'Save' }}
        </AppButton>
      </div>
    </div>
  </div>
</template>

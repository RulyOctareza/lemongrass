<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The HTTP channel creation form, used by the HTTP Channels section and by a rester ask. Its styles
// come from vault.css, so it must render inside a .vault-tab element. A parent reopens it with fresh
// values by remounting it under a new key.
import { onMounted, ref, toRef } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type {
  VaultEnvironment,
  VaultHTTPChannel,
  VaultHTTPChannelWithShortId,
  VaultHTTPScope,
  VaultRester,
  VaultWebScope
} from '../../../../preload/types'
import AppButton from '../AppButton.vue'
import AppCheckbox from '../AppCheckbox.vue'
import AppSegmented from '../AppSegmented.vue'
import AppSwitcher from '../AppSwitcher.vue'

const props = defineProps<{
  resters: string[]
  title: string
  // The channel a Copy starts from.
  seed?: VaultHTTPChannel
  copiedFrom?: string
}>()

const emit = defineEmits<{
  created: [result: VaultHTTPChannelWithShortId]
  cancel: []
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

type ChannelType = 'api' | 'web' | 'app'

const typeLabels: Record<ChannelType, string> = { api: 'API', web: 'Web', app: 'App' }

// The picked rester, read in full for its environments and sides. A rester is one encrypted
// record, so reading it needs the session passphrase.
const selectedRester = ref<VaultRester | null>(null)
const selectedResterError = ref('')
let resterLoadSeq = 0

// Resolves true when this load is still the latest, so a late stale pick cannot overwrite a newer one.
async function loadSelectedRester(name: string): Promise<boolean> {
  const seq = ++resterLoadSeq
  selectedRester.value = null
  selectedResterError.value = ''
  if (!name || !vaultUnlocked.value) return seq === resterLoadSeq
  try {
    const rester = await window.api.vault.getRester(sessionPassphrase.value, name)
    if (seq === resterLoadSeq) selectedRester.value = rester
  } catch (err) {
    if (seq === resterLoadSeq) selectedResterError.value = describeError(err)
  }
  return seq === resterLoadSeq
}

function pickRester(name: string): void {
  newHTTPChannelRester.value = name
  newHTTPEnvironment.value = ''
  loadSelectedRester(name).then((current) => {
    if (!current) return
    if (!newHTTPEnvironment.value) newHTTPEnvironment.value = firstEnvironment()
    if (!typeAvailable(newHTTPType.value)) newHTTPType.value = firstAvailableType()
  })
}

function firstEnvironment(): string {
  return selectedRester.value?.Environments?.[0]?.Name ?? ''
}

function selectedEnvironment(): VaultEnvironment | null {
  const name = newHTTPEnvironment.value
  return selectedRester.value?.Environments?.find((e) => e.Name === name) ?? null
}

// Why a type cannot be picked in the current environment, empty when it can.
function typeUnavailableReason(type: ChannelType): string {
  const rester = selectedRester.value
  const env = selectedEnvironment()
  if (!rester || !env) return ''
  if (type === 'api') {
    if (!rester.API) return `${newHTTPChannelRester.value} has no API side.`
    if (!env.BaseURL) return `Environment ${env.Name} has no base URL.`
    return ''
  }
  if (type === 'app') {
    return rester.App ? '' : `${newHTTPChannelRester.value} has no App side.`
  }
  if (!rester.Web) return `${newHTTPChannelRester.value} has no Web side.`
  if (!env.Origin) return `Environment ${env.Name} has no web origin.`
  return ''
}

function typeAvailable(type: ChannelType): boolean {
  return selectedEnvironment() !== null && typeUnavailableReason(type) === ''
}

function firstAvailableType(): ChannelType {
  const types: ChannelType[] = ['api', 'web', 'app']
  return types.find(typeAvailable) ?? 'api'
}

function pickEnvironment(name: string): void {
  newHTTPEnvironment.value = name
  if (!typeAvailable(newHTTPType.value)) newHTTPType.value = firstAvailableType()
}

// A fixed set, since there is no schema to read methods from.
const methodOptions = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'] as const

interface ExclusionDraft {
  method: string
  pathPattern: string
}

const newHTTPChannelName = ref('')
const newHTTPChannelRester = ref('')
const newHTTPEnvironment = ref('')
const newHTTPType = ref<ChannelType>('api')
const newHTTPMethods = ref<string[]>([])
const newHTTPOrigins = ref<string[]>([])
const newHTTPExclusions = ref<ExclusionDraft[]>([])
const newHTTPTtlMinutes = ref(10)
const creatingHTTPChannel = ref(false)
const createHTTPError = ref('')

function toggleMethod(method: string): void {
  const i = newHTTPMethods.value.indexOf(method)
  if (i === -1) newHTTPMethods.value.push(method)
  else newHTTPMethods.value.splice(i, 1)
}

function addExclusion(): void {
  newHTTPExclusions.value.push({ method: newHTTPMethods.value[0] ?? 'GET', pathPattern: '' })
}

function addOrigin(): void {
  newHTTPOrigins.value.push('')
}

function removeOrigin(index: number): void {
  newHTTPOrigins.value.splice(index, 1)
}

function removeExclusion(index: number): void {
  newHTTPExclusions.value.splice(index, 1)
}

async function submitHTTPCreate(): Promise<void> {
  if (
    !newHTTPChannelName.value.trim() ||
    !newHTTPChannelRester.value.trim() ||
    !typeAvailable(newHTTPType.value) ||
    !vaultUnlocked.value ||
    creatingHTTPChannel.value
  )
    return
  creatingHTTPChannel.value = true
  createHTTPError.value = ''
  try {
    const exclusions = newHTTPExclusions.value
      .filter((e) => e.pathPattern.trim())
      .map((e) => ({ Method: e.method, PathPattern: e.pathPattern.trim() }))
    const web = newHTTPType.value === 'web'
    // A channel carries only its own type's scope, and an app channel has none.
    const scope: VaultHTTPScope =
      newHTTPType.value !== 'api'
        ? { Methods: [], Exclusions: [] }
        : { Methods: [...newHTTPMethods.value], Exclusions: exclusions }
    const webScope: VaultWebScope | null = web
      ? {
          Origins: newHTTPOrigins.value.map((o) => o.trim()).filter(Boolean),
          Exclusions: exclusions
        }
      : null
    const result = await window.api.vault.createHTTPChannel(
      sessionPassphrase.value,
      newHTTPChannelName.value.trim(),
      newHTTPChannelRester.value.trim(),
      newHTTPEnvironment.value,
      newHTTPType.value,
      scope,
      webScope,
      Math.round(newHTTPTtlMinutes.value * 60)
    )
    emit('created', result)
  } catch (err) {
    createHTTPError.value = describeError(err)
  } finally {
    creatingHTTPChannel.value = false
  }
}

// Applied once at mount: a copy carries the source channel's scope, a fresh form starts empty.
function applySeed(): void {
  const channel = props.seed
  if (!channel) {
    pickRester(props.resters[0] ?? '')
    return
  }
  newHTTPChannelName.value = `${props.copiedFrom || 'channel'} copy`
  const type: ChannelType = channel.Type || 'api'
  const exclusions =
    type === 'web' ? (channel.WebScope?.Exclusions ?? []) : (channel.Scope.Exclusions ?? [])
  newHTTPType.value = type
  newHTTPMethods.value = [...(channel.Scope.Methods ?? [])]
  newHTTPOrigins.value = [...(channel.WebScope?.Origins ?? [])]
  newHTTPExclusions.value = exclusions.map((e) => ({
    method: e.Method,
    pathPattern: e.PathPattern
  }))
  const resterName = props.resters.includes(channel.Rester) ? channel.Rester : ''
  newHTTPChannelRester.value = resterName
  loadSelectedRester(resterName).then((current) => {
    if (!current) return
    const wanted = channel.Environment || 'default'
    const found = selectedRester.value?.Environments?.some((e) => e.Name === wanted)
    newHTTPEnvironment.value = found ? wanted : firstEnvironment()
    if (!typeAvailable(newHTTPType.value)) newHTTPType.value = firstAvailableType()
  })
}

onMounted(applySeed)
</script>

<template>
  <div class="inline-section">
    <h4 class="inline-section-title">{{ title }}</h4>

    <div class="form-columns">
      <div class="form-column">
        <div class="field-block">
          <label class="field-label" for="new-http-channel-name">Name</label>
          <input
            id="new-http-channel-name"
            v-model="newHTTPChannelName"
            type="text"
            placeholder="e.g. debug X"
          />
        </div>
        <div class="field-block">
          <label class="field-label">Rester</label>
          <AppSwitcher
            framed
            :items="resters.map((name) => ({ id: name, label: name }))"
            :active-id="newHTTPChannelRester || null"
            placeholder="Pick a rester"
            empty-text="No resters yet."
            @select="pickRester"
          />
          <p v-if="selectedResterError" class="error-text">{{ selectedResterError }}</p>
        </div>
        <div class="field-block">
          <label class="field-label">Environment</label>
          <AppSegmented
            v-if="selectedRester?.Environments?.length"
            aria-label="Environment"
            :options="selectedRester.Environments.map((e) => ({ value: e.Name, label: e.Name }))"
            :model-value="newHTTPEnvironment"
            @update:model-value="pickEnvironment"
          />
          <p v-else class="hint">
            {{ newHTTPChannelRester ? 'This rester has no environments.' : 'Pick a rester first.' }}
          </p>
        </div>
        <div class="field-block">
          <label class="field-label">Type</label>
          <div class="type-choice">
            <AppButton
              v-for="t in ['api', 'web', 'app'] as const"
              :key="t"
              type="button"
              :variant="newHTTPType === t ? 'primary' : 'ghost'"
              :disabled="!typeAvailable(t)"
              :title="typeUnavailableReason(t)"
              @click="newHTTPType = t"
            >
              {{ typeLabels[t] }}
            </AppButton>
          </div>
          <p v-if="typeUnavailableReason('api')" class="hint">
            API is unavailable: {{ typeUnavailableReason('api') }}
          </p>
          <p v-if="typeUnavailableReason('web')" class="hint">
            Web is unavailable: {{ typeUnavailableReason('web') }}
          </p>
          <p v-if="typeUnavailableReason('app')" class="hint">
            App is unavailable: {{ typeUnavailableReason('app') }}
          </p>
        </div>
        <div class="field-block">
          <label class="field-label" for="new-http-channel-ttl">TTL</label>
          <input
            id="new-http-channel-ttl"
            v-model.number="newHTTPTtlMinutes"
            type="number"
            min="1"
            @keydown.enter="submitHTTPCreate"
          />
          <p class="hint">
            Channel stays open for {{ newHTTPTtlMinutes }} minutes before it needs reactivating.
          </p>
        </div>
      </div>
      <div class="form-column">
        <div v-if="newHTTPType === 'app'" class="field-block">
          <p class="hint">
            An app channel logs the phone in through the rester's app login steps. It needs a login
            user that has every field those steps fill, and it has no scope to set.
          </p>
        </div>
        <div v-else-if="newHTTPType === 'api'" class="field-block">
          <label class="field-label">Methods</label>
          <div class="checkbox-grid">
            <AppCheckbox
              v-for="m in methodOptions"
              :key="m"
              :checked="newHTTPMethods.includes(m)"
              @click="toggleMethod(m)"
            >
              {{ m }}
            </AppCheckbox>
          </div>
        </div>
        <div v-else class="field-block">
          <label class="field-label">Extra origins</label>
          <p class="hint">
            The environment's own origin is always allowed. Add another one, such as a single
            sign-on host.
          </p>
          <div v-for="(_origin, i) in newHTTPOrigins" :key="i" class="exclusion-row">
            <input v-model="newHTTPOrigins[i]" type="text" placeholder="https://sso.example.com" />
            <button
              type="button"
              class="chip-remove"
              title="Remove origin"
              @click="removeOrigin(i)"
            >
              ×
            </button>
          </div>
          <AppButton type="button" @click="addOrigin">+ Origin</AppButton>
        </div>
        <div v-if="newHTTPType !== 'app'" class="field-block">
          <label class="field-label">Exclusions</label>
          <p class="hint">
            {{
              newHTTPType === 'api'
                ? 'Overrides a granted method for one specific path pattern.'
                : 'Blocks requests the page sends to the environment for one method and path pattern.'
            }}
          </p>
          <div v-for="(ex, i) in newHTTPExclusions" :key="i" class="exclusion-row">
            <AppSwitcher
              framed
              :items="methodOptions.map((m) => ({ id: m, label: m }))"
              :active-id="ex.method"
              placeholder="Method"
              empty-text="No methods."
              @select="(m) => (ex.method = m)"
            />
            <input v-model="ex.pathPattern" type="text" placeholder="/users/*/change-password" />
            <button
              type="button"
              class="chip-remove"
              title="Remove exclusion"
              @click="removeExclusion(i)"
            >
              ×
            </button>
          </div>
          <AppButton type="button" @click="addExclusion">+ Exclusion</AppButton>
        </div>
      </div>
    </div>

    <p v-if="createHTTPError" class="error-text">{{ createHTTPError }}</p>
    <div class="create-actions">
      <AppButton @click="emit('cancel')">Cancel</AppButton>
      <AppButton
        variant="primary"
        :disabled="
          !newHTTPChannelName.trim() ||
          !newHTTPChannelRester.trim() ||
          !typeAvailable(newHTTPType) ||
          creatingHTTPChannel
        "
        @click="submitHTTPCreate"
      >
        {{ creatingHTTPChannel ? 'Creating...' : 'Create' }}
      </AppButton>
    </div>
  </div>
</template>

<style scoped>
.type-choice {
  display: flex;
  gap: var(--space-2);
}
</style>

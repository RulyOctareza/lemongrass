<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type {
  VaultEnvironment,
  VaultHTTPChannel,
  VaultHTTPScope,
  VaultRester,
  VaultWebScope
} from '../../../../preload/types'
import AppButton from '../AppButton.vue'
import AppCheckbox from '../AppCheckbox.vue'
import AppSegmented from '../AppSegmented.vue'
import AppSwitcher from '../AppSwitcher.vue'

const props = defineProps<{
  visible: boolean
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

const resters = ref<string[]>([])

async function refreshResters(): Promise<void> {
  try {
    resters.value = await window.api.vault.listResters()
  } catch (err) {
    httpChannelsListError.value = describeError(err)
  }
}

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

const httpChannels = ref<VaultHTTPChannel[]>([])
const httpChannelsLoading = ref(false)
const httpChannelsListError = ref('')
const shortIdByHTTPChannel = toRef(vaultSession, 'shortIdByHTTPChannel')

async function refreshHTTPChannels(): Promise<void> {
  httpChannelsLoading.value = true
  httpChannelsListError.value = ''
  try {
    httpChannels.value = await window.api.vault.listHTTPChannels()
  } catch (err) {
    httpChannelsListError.value = describeError(err)
  } finally {
    httpChannelsLoading.value = false
  }
}

// A fixed set, since there is no schema to read methods from.
const methodOptions = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'] as const

interface ExclusionDraft {
  method: string
  pathPattern: string
}

const showHTTPCreateForm = ref(false)
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
const copiedFrom = ref('')
const rootEl = ref<HTMLElement | null>(null)

function openHTTPCreateForm(): void {
  copiedFrom.value = ''
  newHTTPChannelName.value = ''
  newHTTPType.value = 'api'
  newHTTPMethods.value = []
  newHTTPOrigins.value = []
  newHTTPExclusions.value = []
  newHTTPTtlMinutes.value = 10
  createHTTPError.value = ''
  showHTTPCreateForm.value = true
  pickRester(resters.value[0] ?? '')
}

function openHTTPCopyForm(channel: VaultHTTPChannel): void {
  copiedFrom.value = channel.Name || 'channel'
  newHTTPChannelName.value = `${copiedFrom.value} copy`
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
  newHTTPTtlMinutes.value = 10
  createHTTPError.value = ''
  showHTTPCreateForm.value = true
  const resterName = resters.value.includes(channel.Rester) ? channel.Rester : ''
  newHTTPChannelRester.value = resterName
  newHTTPEnvironment.value = ''
  loadSelectedRester(resterName).then((current) => {
    if (!current) return
    const wanted = channel.Environment || 'default'
    const found = selectedRester.value?.Environments?.some((e) => e.Name === wanted)
    newHTTPEnvironment.value = found ? wanted : firstEnvironment()
    if (!typeAvailable(newHTTPType.value)) newHTTPType.value = firstAvailableType()
  })
  nextTick(() => rootEl.value?.closest('.vault-main')?.scrollTo({ top: 0, behavior: 'smooth' }))
}

function cancelHTTPCreate(): void {
  showHTTPCreateForm.value = false
}

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
    const { channel, shortId } = await window.api.vault.createHTTPChannel(
      sessionPassphrase.value,
      newHTTPChannelName.value.trim(),
      newHTTPChannelRester.value.trim(),
      newHTTPEnvironment.value,
      newHTTPType.value,
      scope,
      webScope,
      Math.round(newHTTPTtlMinutes.value * 60)
    )
    shortIdByHTTPChannel.value[channel.ID] = shortId
    showHTTPCreateForm.value = false
    await refreshHTTPChannels()
  } catch (err) {
    createHTTPError.value = describeError(err)
  } finally {
    creatingHTTPChannel.value = false
  }
}

const activatingHTTPId = ref<string | null>(null)
const activateHTTPTtlMinutes = ref(10)
const activatingHTTP = ref(false)
const activateHTTPError = ref('')

function openHTTPActivate(id: string): void {
  activatingHTTPId.value = id
  activateHTTPTtlMinutes.value = 10
  activateHTTPError.value = ''
}

function cancelHTTPActivate(): void {
  activatingHTTPId.value = null
}

async function submitHTTPActivate(): Promise<void> {
  if (!activatingHTTPId.value || !vaultUnlocked.value || activatingHTTP.value) return
  activatingHTTP.value = true
  activateHTTPError.value = ''
  const id = activatingHTTPId.value
  try {
    const { channel, shortId } = await window.api.vault.activateHTTPChannel(
      sessionPassphrase.value,
      id,
      Math.round(activateHTTPTtlMinutes.value * 60)
    )
    shortIdByHTTPChannel.value[channel.ID] = shortId
    activatingHTTPId.value = null
    await refreshHTTPChannels()
  } catch (err) {
    activateHTTPError.value = describeError(err)
  } finally {
    activatingHTTP.value = false
  }
}

const revokingHTTPId = ref<string | null>(null)

async function revokeHTTP(id: string): Promise<void> {
  revokingHTTPId.value = id
  try {
    await window.api.vault.revokeHTTPChannel(id)
    delete shortIdByHTTPChannel.value[id]
    await refreshHTTPChannels()
  } catch (err) {
    httpChannelsListError.value = describeError(err)
  } finally {
    revokingHTTPId.value = null
  }
}

function statusForHTTP(channel: VaultHTTPChannel): string {
  const expires = new Date(channel.ExpiresAt)
  if (Number.isNaN(expires.getTime()) || expires.getTime() <= Date.now()) return 'expired'
  const hh = String(expires.getHours()).padStart(2, '0')
  const mm = String(expires.getMinutes()).padStart(2, '0')
  return `open until ${hh}:${mm}`
}

const copiedHTTPChannelId = ref<string | null>(null)
let copiedHTTPTimer: ReturnType<typeof setTimeout> | null = null

async function copyHTTPShortId(id: string): Promise<void> {
  const value = shortIdByHTTPChannel.value[id]
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
  } catch {
    return
  }
  copiedHTTPChannelId.value = id
  if (copiedHTTPTimer) clearTimeout(copiedHTTPTimer)
  copiedHTTPTimer = setTimeout(() => {
    copiedHTTPChannelId.value = null
  }, 1500)
}

onMounted(() => {
  refreshResters()
  refreshHTTPChannels()
})

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    refreshResters()
    refreshHTTPChannels()
  }
)

onUnmounted(() => {
  if (copiedHTTPTimer) clearTimeout(copiedHTTPTimer)
})
</script>

<template>
  <div ref="rootEl" class="section">
    <div class="section-toolbar">
      <p class="hint">
        Scoped, short-lived access to a rester environment. The newest channel is first.
      </p>
      <AppButton
        v-if="!showHTTPCreateForm"
        variant="primary"
        class="new-channel-button"
        :disabled="resters.length === 0"
        :title="resters.length === 0 ? 'Add a rester first' : ''"
        @click="openHTTPCreateForm"
      >
        + New channel
      </AppButton>
    </div>

    <div v-if="showHTTPCreateForm" class="inline-section">
      <h4 class="inline-section-title">
        {{ copiedFrom ? `New HTTP channel from ${copiedFrom}` : 'New HTTP channel' }}
      </h4>

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
              {{
                newHTTPChannelRester ? 'This rester has no environments.' : 'Pick a rester first.'
              }}
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
              An app channel logs the phone in through the rester's app login steps. It needs a
              login user that has every field those steps fill, and it has no scope to set.
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
              <input
                v-model="newHTTPOrigins[i]"
                type="text"
                placeholder="https://sso.example.com"
              />
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
        <AppButton @click="cancelHTTPCreate">Cancel</AppButton>
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

    <p v-if="httpChannelsListError" class="error-text">{{ httpChannelsListError }}</p>
    <p v-else-if="httpChannelsLoading" class="empty">Loading...</p>
    <p v-else-if="httpChannels.length === 0" class="empty">No HTTP channels yet.</p>

    <div v-else class="channel-list">
      <div
        v-for="channel in httpChannels"
        :key="channel.ID"
        class="channel-row channel-row--stacked"
      >
        <div class="channel-top">
          <div class="channel-info">
            <span class="channel-name">{{
              channel.Name || shortIdByHTTPChannel[channel.ID] || channel.ID
            }}</span>
            <span class="channel-db">{{ channel.Rester }}</span>
            <div class="capsule-row">
              <span class="value-capsule">{{ channel.Environment || 'default' }}</span>
              <span class="value-capsule">{{ channel.Type || 'api' }}</span>
            </div>
            <div v-if="channel.Type === 'web'" class="capsule-group">
              <span class="capsule-group-label">Origins</span>
              <div class="capsule-row">
                <span class="value-capsule">environment origin</span>
                <span v-for="o in channel.WebScope?.Origins || []" :key="o" class="value-capsule">
                  {{ o }}
                </span>
              </div>
            </div>
            <div
              v-if="channel.Type === 'web' && channel.WebScope?.Exclusions?.length"
              class="capsule-group"
            >
              <span class="capsule-group-label">Exclusions</span>
              <div class="capsule-row">
                <span v-for="(ex, i) in channel.WebScope.Exclusions" :key="i" class="value-capsule">
                  {{ ex.Method }} {{ ex.PathPattern }}
                </span>
              </div>
            </div>
            <div v-if="channel.Type === 'api' || !channel.Type" class="capsule-group">
              <span class="capsule-group-label">Methods</span>
              <div class="capsule-row">
                <span v-for="m in channel.Scope.Methods || []" :key="m" class="value-capsule">
                  {{ m }}
                </span>
                <span v-if="!channel.Scope.Methods?.length" class="value-capsule">none</span>
              </div>
            </div>
            <div
              v-if="(channel.Type === 'api' || !channel.Type) && channel.Scope.Exclusions?.length"
              class="capsule-group"
            >
              <span class="capsule-group-label">Exclusions</span>
              <div class="capsule-row">
                <span v-for="(ex, i) in channel.Scope.Exclusions" :key="i" class="value-capsule">
                  {{ ex.Method }} {{ ex.PathPattern }}
                </span>
              </div>
            </div>
          </div>

          <span
            v-if="shortIdByHTTPChannel[channel.ID]"
            class="channel-code"
            :title="
              copiedHTTPChannelId === channel.ID
                ? 'Copied.'
                : 'Click to copy. Paste it into the agent to use this channel.'
            "
            @click="copyHTTPShortId(channel.ID)"
            >{{ shortIdByHTTPChannel[channel.ID] }}</span
          >
        </div>

        <div class="channel-bottom">
          <span
            class="status-capsule"
            :class="statusForHTTP(channel) === 'expired' ? 'status-expired' : 'status-open'"
          >
            {{ statusForHTTP(channel) }}
          </span>
          <div v-if="activatingHTTPId !== channel.ID" class="channel-actions">
            <AppButton
              :disabled="!vaultUnlocked"
              title="Create a new channel from this one"
              @click="openHTTPCopyForm(channel)"
            >
              Copy
            </AppButton>
            <AppButton @click="openHTTPActivate(channel.ID)">
              {{ statusForHTTP(channel) === 'expired' ? 'Activate' : 'Rotate' }}
            </AppButton>
            <AppButton
              variant="danger"
              :disabled="revokingHTTPId === channel.ID"
              @click="revokeHTTP(channel.ID)"
            >
              {{ revokingHTTPId === channel.ID ? 'Removing...' : 'Remove' }}
            </AppButton>
          </div>
        </div>

        <div v-if="activatingHTTPId === channel.ID" class="inline-form">
          <input
            v-model.number="activateHTTPTtlMinutes"
            type="number"
            min="1"
            title="TTL, minutes"
            autofocus
            @keydown.enter="submitHTTPActivate"
          />
          <AppButton @click="cancelHTTPActivate">Cancel</AppButton>
          <AppButton variant="primary" :disabled="activatingHTTP" @click="submitHTTPActivate">
            {{
              statusForHTTP(channel) === 'expired'
                ? activatingHTTP
                  ? 'Activating...'
                  : 'Activate'
                : activatingHTTP
                  ? 'Rotating...'
                  : 'Rotate'
            }}
          </AppButton>
        </div>
        <p v-if="activatingHTTPId === channel.ID" class="hint">
          Channel stays open for {{ activateHTTPTtlMinutes }} minutes before it needs reactivating.
        </p>
        <p v-if="activatingHTTPId === channel.ID && activateHTTPError" class="error-text">
          {{ activateHTTPError }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.type-choice {
  display: flex;
  gap: var(--space-2);
}
</style>

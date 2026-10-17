<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type { VaultHTTPChannel, VaultHTTPScope } from '../../../../preload/types'
import AppButton from '../AppButton.vue'

const props = defineProps<{
  visible: boolean
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

const domains = ref<string[]>([])

async function refreshDomains(): Promise<void> {
  try {
    domains.value = await window.api.vault.listDomains()
  } catch (err) {
    httpChannelsListError.value = describeError(err)
  }
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
const newHTTPChannelDomain = ref('')
const newHTTPMethods = ref<string[]>([])
const newHTTPExclusions = ref<ExclusionDraft[]>([])
const newHTTPTtlMinutes = ref(10)
const creatingHTTPChannel = ref(false)
const createHTTPError = ref('')
const copiedFrom = ref('')
const rootEl = ref<HTMLElement | null>(null)

function openHTTPCreateForm(): void {
  copiedFrom.value = ''
  newHTTPChannelName.value = ''
  newHTTPChannelDomain.value = domains.value[0] ?? ''
  newHTTPMethods.value = []
  newHTTPExclusions.value = []
  newHTTPTtlMinutes.value = 10
  createHTTPError.value = ''
  showHTTPCreateForm.value = true
}

function openHTTPCopyForm(channel: VaultHTTPChannel): void {
  copiedFrom.value = channel.Name || 'channel'
  newHTTPChannelName.value = `${copiedFrom.value} copy`
  newHTTPChannelDomain.value = domains.value.includes(channel.Domain) ? channel.Domain : ''
  newHTTPMethods.value = [...(channel.Scope.Methods ?? [])]
  newHTTPExclusions.value = (channel.Scope.Exclusions ?? []).map((e) => ({
    method: e.Method,
    pathPattern: e.PathPattern
  }))
  newHTTPTtlMinutes.value = 10
  createHTTPError.value = ''
  showHTTPCreateForm.value = true
  nextTick(() => rootEl.value?.closest('.vault-main')?.scrollTo({ top: 0, behavior: 'smooth' }))
}

function cancelHTTPCreate(): void {
  showHTTPCreateForm.value = false
}

function addExclusion(): void {
  newHTTPExclusions.value.push({ method: newHTTPMethods.value[0] ?? 'GET', pathPattern: '' })
}

function removeExclusion(index: number): void {
  newHTTPExclusions.value.splice(index, 1)
}

async function submitHTTPCreate(): Promise<void> {
  if (
    !newHTTPChannelName.value.trim() ||
    !newHTTPChannelDomain.value.trim() ||
    !vaultUnlocked.value ||
    creatingHTTPChannel.value
  )
    return
  creatingHTTPChannel.value = true
  createHTTPError.value = ''
  try {
    const scope: VaultHTTPScope = {
      Methods: [...newHTTPMethods.value],
      Exclusions: newHTTPExclusions.value
        .filter((e) => e.pathPattern.trim())
        .map((e) => ({ Method: e.method, PathPattern: e.pathPattern.trim() }))
    }
    const { channel, shortId } = await window.api.vault.createHTTPChannel(
      sessionPassphrase.value,
      newHTTPChannelName.value.trim(),
      newHTTPChannelDomain.value.trim(),
      scope,
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
  refreshDomains()
  refreshHTTPChannels()
})

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    refreshDomains()
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
      <p class="hint">Scoped, short-lived access to a domain. The newest channel is first.</p>
      <AppButton
        v-if="!showHTTPCreateForm"
        variant="primary"
        class="new-channel-button"
        :disabled="domains.length === 0"
        :title="domains.length === 0 ? 'Add a domain first' : ''"
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
            <label class="field-label" for="new-http-channel-domain">Domain</label>
            <select id="new-http-channel-domain" v-model="newHTTPChannelDomain">
              <option v-for="name in domains" :key="name" :value="name">{{ name }}</option>
            </select>
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
          <div class="field-block">
            <label class="field-label">Methods</label>
            <div class="checkbox-grid">
              <label v-for="m in methodOptions" :key="m" class="checkbox-option">
                <input v-model="newHTTPMethods" type="checkbox" :value="m" />
                {{ m }}
              </label>
            </div>
          </div>
          <div class="field-block">
            <label class="field-label">Exclusions</label>
            <p class="hint">Overrides a granted method for one specific path pattern.</p>
            <div v-for="(ex, i) in newHTTPExclusions" :key="i" class="exclusion-row">
              <select v-model="ex.method">
                <option v-for="m in methodOptions" :key="m" :value="m">{{ m }}</option>
              </select>
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
            !newHTTPChannelName.trim() || !newHTTPChannelDomain.trim() || creatingHTTPChannel
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
            <span class="channel-db">{{ channel.Domain }}</span>
            <div class="capsule-group">
              <span class="capsule-group-label">Methods</span>
              <div class="capsule-row">
                <span v-for="m in channel.Scope.Methods || []" :key="m" class="value-capsule">
                  {{ m }}
                </span>
                <span v-if="!channel.Scope.Methods?.length" class="value-capsule">none</span>
              </div>
            </div>
            <div v-if="channel.Scope.Exclusions?.length" class="capsule-group">
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

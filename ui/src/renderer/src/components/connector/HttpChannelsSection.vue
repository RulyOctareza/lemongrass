<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type {
  VaultHTTPChannel,
  VaultHTTPChannelWithShortId,
  VaultResterSummary
} from '../../../../preload/types'
import AppButton from '../AppButton.vue'
import HttpChannelForm from './HttpChannelForm.vue'
import ChannelTypeIcon from './ChannelTypeIcon.vue'
import AppSwitcher from '../AppSwitcher.vue'
import { matchesQuery, resterDomains } from './listFilter'
import AppInput from '../AppInput.vue'

const props = defineProps<{
  visible: boolean
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

const resters = ref<string[]>([])
const summaries = ref<Record<string, VaultResterSummary>>({})

async function refreshResters(): Promise<void> {
  try {
    resters.value = await window.api.vault.listResters()
  } catch (err) {
    httpChannelsListError.value = describeError(err)
  }
}

// Domains live inside the encrypted rester; without them the filter still matches everything else.
async function refreshSummaries(): Promise<void> {
  try {
    const list = await window.api.vault.listResterSummaries(sessionPassphrase.value)
    summaries.value = Object.fromEntries(list.map((s) => [s.Name, s]))
  } catch {
    summaries.value = {}
  }
}

// A rester name is never empty, so the empty id cannot collide with one.
const ALL_RESTERS = ''
const resterFilter = ref(ALL_RESTERS)
const query = ref('')

watch(resters, (names) => {
  if (resterFilter.value !== ALL_RESTERS && !names.includes(resterFilter.value)) {
    resterFilter.value = ALL_RESTERS
  }
})

const resterItems = computed(() => [
  { id: ALL_RESTERS, label: 'All resters' },
  ...resters.value.map((name) => ({
    id: name,
    label: name,
    search: [name, ...resterDomains(summaries.value[name])].join(' ')
  }))
])

const shownChannels = computed(() =>
  httpChannels.value.filter((channel) => {
    if (resterFilter.value !== ALL_RESTERS && channel.Rester !== resterFilter.value) return false
    return matchesQuery(query.value, [
      channel.Name,
      channel.Rester,
      channel.Type || 'api',
      ...(channel.Scope.Methods ?? []),
      ...resterDomains(summaries.value[channel.Rester], channel.Environment)
    ])
  })
)

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

const showHTTPCreateForm = ref(false)
const formKey = ref(0)
const formSeed = ref<VaultHTTPChannel | undefined>(undefined)
const copiedFrom = ref('')
const rootEl = ref<HTMLElement | null>(null)

function openHTTPCreateForm(): void {
  copiedFrom.value = ''
  formSeed.value = undefined
  formKey.value++
  showHTTPCreateForm.value = true
}

function openHTTPCopyForm(channel: VaultHTTPChannel): void {
  copiedFrom.value = channel.Name || 'channel'
  formSeed.value = channel
  formKey.value++
  showHTTPCreateForm.value = true
  nextTick(() => rootEl.value?.closest('.vault-main')?.scrollTo({ top: 0, behavior: 'smooth' }))
}

function cancelHTTPCreate(): void {
  showHTTPCreateForm.value = false
}

async function onHTTPCreated(result: VaultHTTPChannelWithShortId): Promise<void> {
  shortIdByHTTPChannel.value[result.channel.ID] = result.shortId
  showHTTPCreateForm.value = false
  await refreshHTTPChannels()
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
  refreshSummaries()
  refreshHTTPChannels()
})

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    refreshResters()
    refreshSummaries()
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

    <HttpChannelForm
      v-if="showHTTPCreateForm"
      :key="formKey"
      :resters="resters"
      :title="copiedFrom ? `New HTTP channel from ${copiedFrom}` : 'New HTTP channel'"
      :seed="formSeed"
      :copied-from="copiedFrom"
      @created="onHTTPCreated"
      @cancel="cancelHTTPCreate"
    />

    <p v-if="httpChannelsListError" class="error-text">{{ httpChannelsListError }}</p>
    <p v-else-if="httpChannelsLoading" class="empty">Loading...</p>
    <p v-else-if="httpChannels.length === 0" class="empty">No HTTP channels yet.</p>

    <template v-else>
      <div class="list-filter">
        <AppSwitcher
          class="rester-filter"
          :items="resterItems"
          :active-id="resterFilter"
          placeholder="All resters"
          empty-text="No resters."
          framed
          searchable
          search-placeholder="Search resters"
          @select="resterFilter = $event"
        />
        <AppInput
          v-model="query"
          type="text"
          spellcheck="false"
          icon="search"
          placeholder="Filter by name, rester, method, type or domain"
        />
      </div>
      <p v-if="shownChannels.length === 0" class="empty">No channel matches.</p>
    </template>

    <div
      v-if="shownChannels.length > 0 && !httpChannelsListError && !httpChannelsLoading"
      class="channel-list"
    >
      <div
        v-for="channel in shownChannels"
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
              <span class="value-capsule type-capsule">
                <ChannelTypeIcon :type="channel.Type || 'api'" />
                {{ channel.Type || 'api' }}
              </span>
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
          <AppInput
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

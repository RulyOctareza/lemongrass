<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type { VaultChannel, VaultChannelWithShortId } from '../../../../preload/types'
import AppButton from '../AppButton.vue'
import DbChannelForm, { type DbChannelSeed } from './DbChannelForm.vue'
import AppInput from '../AppInput.vue'
import AppSwitcher from '../AppSwitcher.vue'
import { matchesQuery } from './listFilter'

const props = defineProps<{
  visible: boolean
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

const connections = ref<string[]>([])

async function refreshConnections(): Promise<void> {
  try {
    connections.value = await window.api.vault.listConnections()
  } catch (err) {
    listError.value = describeError(err)
  }
}

const channels = ref<VaultChannel[]>([])
const loading = ref(false)
const listError = ref('')
const shortIdByChannel = toRef(vaultSession, 'shortIdByChannel')

async function refresh(): Promise<void> {
  loading.value = true
  listError.value = ''
  try {
    channels.value = await window.api.vault.list()
  } catch (err) {
    listError.value = describeError(err)
  } finally {
    loading.value = false
  }
}

// A connection name is never empty, so the empty id cannot collide with one.
const ALL_CONNECTIONS = ''
const connectionFilter = ref(ALL_CONNECTIONS)
const query = ref('')

watch(connections, (names) => {
  if (connectionFilter.value !== ALL_CONNECTIONS && !names.includes(connectionFilter.value)) {
    connectionFilter.value = ALL_CONNECTIONS
  }
})

const connectionItems = computed(() => [
  { id: ALL_CONNECTIONS, label: 'All connections' },
  ...connections.value.map((name) => ({ id: name, label: name }))
])

const shownChannels = computed(() =>
  channels.value.filter((channel) => {
    if (connectionFilter.value !== ALL_CONNECTIONS && channel.DBName !== connectionFilter.value) {
      return false
    }
    return matchesQuery(query.value, [
      channel.Name,
      ...(channel.Scope.Tables ?? []),
      ...(channel.Scope.Operations ?? [])
    ])
  })
)

function statusFor(channel: VaultChannel): string {
  const expires = new Date(channel.ExpiresAt)
  if (Number.isNaN(expires.getTime()) || expires.getTime() <= Date.now()) return 'expired'
  const hh = String(expires.getHours()).padStart(2, '0')
  const mm = String(expires.getMinutes()).padStart(2, '0')
  return `open until ${hh}:${mm}`
}

const copiedChannelId = ref<string | null>(null)
let copiedTimer: ReturnType<typeof setTimeout> | null = null

async function copyShortId(id: string): Promise<void> {
  const value = shortIdByChannel.value[id]
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
  } catch {
    return
  }
  copiedChannelId.value = id
  if (copiedTimer) clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => {
    copiedChannelId.value = null
  }, 1500)
}

const showCreateForm = ref(false)
const formKey = ref(0)
const formSeed = ref<DbChannelSeed | undefined>(undefined)
const copiedFrom = ref('')
const rootEl = ref<HTMLElement | null>(null)

function openCreateForm(): void {
  copiedFrom.value = ''
  formSeed.value = undefined
  formKey.value++
  showCreateForm.value = true
}

function openCopyForm(channel: VaultChannel): void {
  copiedFrom.value = channel.Name || 'channel'
  formSeed.value = {
    name: `${copiedFrom.value} copy`,
    dbName: connections.value.includes(channel.DBName) ? channel.DBName : '',
    tables: [...(channel.Scope.Tables ?? [])],
    operations: [...(channel.Scope.Operations ?? [])]
  }
  formKey.value++
  showCreateForm.value = true
  nextTick(() => rootEl.value?.closest('.vault-main')?.scrollTo({ top: 0, behavior: 'smooth' }))
}

function cancelCreate(): void {
  showCreateForm.value = false
}

async function onCreated(result: VaultChannelWithShortId): Promise<void> {
  shortIdByChannel.value[result.channel.ID] = result.shortId
  showCreateForm.value = false
  await refresh()
}

const activatingId = ref<string | null>(null)
const activateTtlMinutes = ref(10)
const activating = ref(false)
const activateError = ref('')

function openActivate(id: string): void {
  activatingId.value = id
  activateTtlMinutes.value = 10
  activateError.value = ''
}

function cancelActivate(): void {
  activatingId.value = null
}

async function submitActivate(): Promise<void> {
  if (!activatingId.value || !vaultUnlocked.value || activating.value) return
  activating.value = true
  activateError.value = ''
  const id = activatingId.value
  try {
    const { channel, shortId } = await window.api.vault.activate(
      sessionPassphrase.value,
      id,
      Math.round(activateTtlMinutes.value * 60)
    )
    shortIdByChannel.value[channel.ID] = shortId
    activatingId.value = null
    await refresh()
  } catch (err) {
    activateError.value = describeError(err)
  } finally {
    activating.value = false
  }
}

const revokingId = ref<string | null>(null)

async function revoke(id: string): Promise<void> {
  revokingId.value = id
  try {
    await window.api.vault.revoke(id)
    delete shortIdByChannel.value[id]
    await refresh()
  } catch (err) {
    listError.value = describeError(err)
  } finally {
    revokingId.value = null
  }
}

onMounted(() => {
  refreshConnections()
  refresh()
})

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    refreshConnections()
    refresh()
  }
)

onUnmounted(() => {
  if (copiedTimer) clearTimeout(copiedTimer)
})
</script>

<template>
  <div ref="rootEl" class="section">
    <div class="section-toolbar">
      <p class="hint">Scoped, short-lived access to a connection. The newest channel is first.</p>
      <AppButton
        v-if="!showCreateForm"
        variant="primary"
        class="new-channel-button"
        :disabled="connections.length === 0"
        :title="connections.length === 0 ? 'Add a connection first' : ''"
        @click="openCreateForm"
      >
        + New channel
      </AppButton>
    </div>

    <DbChannelForm
      v-if="showCreateForm"
      :key="formKey"
      :connections="connections"
      :title="copiedFrom ? `New channel from ${copiedFrom}` : 'New channel'"
      :seed="formSeed"
      @created="onCreated"
      @cancel="cancelCreate"
    />

    <p v-if="listError" class="error-text">{{ listError }}</p>
    <p v-else-if="loading" class="empty">Loading...</p>
    <p v-else-if="channels.length === 0" class="empty">No channels yet.</p>

    <template v-else>
      <div class="list-filter">
        <AppSwitcher
          class="connection-filter"
          :items="connectionItems"
          :active-id="connectionFilter"
          placeholder="All connections"
          empty-text="No connections."
          framed
          searchable
          search-placeholder="Search connections"
          @select="connectionFilter = $event"
        />
        <AppInput
          v-model="query"
          icon="search"
          spellcheck="false"
          placeholder="Filter by name, table or operation"
        />
      </div>
      <p v-if="shownChannels.length === 0" class="empty">No channel matches.</p>
    </template>

    <div v-if="shownChannels.length > 0 && !listError && !loading" class="channel-list">
      <div
        v-for="channel in shownChannels"
        :key="channel.ID"
        class="channel-row channel-row--stacked"
      >
        <div class="channel-top">
          <div class="channel-info">
            <span class="channel-name">{{
              channel.Name || shortIdByChannel[channel.ID] || channel.ID
            }}</span>
            <span class="channel-db">{{ channel.DBName }}</span>
            <div class="capsule-group">
              <span class="capsule-group-label">Tables</span>
              <div class="capsule-row">
                <span v-for="t in channel.Scope.Tables || []" :key="t" class="value-capsule">
                  {{ t }}
                </span>
                <span v-if="!channel.Scope.Tables?.length" class="value-capsule">none</span>
              </div>
            </div>
            <div class="capsule-group">
              <span class="capsule-group-label">Operations</span>
              <div class="capsule-row">
                <span v-for="op in channel.Scope.Operations || []" :key="op" class="value-capsule">
                  {{ op }}
                </span>
                <span v-if="!channel.Scope.Operations?.length" class="value-capsule">none</span>
              </div>
            </div>
          </div>

          <span
            v-if="shortIdByChannel[channel.ID]"
            class="channel-code"
            :title="
              copiedChannelId === channel.ID
                ? 'Copied.'
                : 'Click to copy. Paste it into the agent to use this channel.'
            "
            @click="copyShortId(channel.ID)"
            >{{ shortIdByChannel[channel.ID] }}</span
          >
        </div>

        <div class="channel-bottom">
          <span
            class="status-capsule"
            :class="statusFor(channel) === 'expired' ? 'status-expired' : 'status-open'"
          >
            {{ statusFor(channel) }}
          </span>
          <div v-if="activatingId !== channel.ID" class="channel-actions">
            <AppButton
              :disabled="!vaultUnlocked"
              title="Create a new channel from this one"
              @click="openCopyForm(channel)"
            >
              Copy
            </AppButton>
            <AppButton @click="openActivate(channel.ID)">
              {{ statusFor(channel) === 'expired' ? 'Activate' : 'Rotate' }}
            </AppButton>
            <AppButton
              variant="danger"
              :disabled="revokingId === channel.ID"
              @click="revoke(channel.ID)"
            >
              {{ revokingId === channel.ID ? 'Removing...' : 'Remove' }}
            </AppButton>
          </div>
        </div>

        <div v-if="activatingId === channel.ID" class="inline-form">
          <AppInput
            v-model.number="activateTtlMinutes"
            type="number"
            min="1"
            title="TTL, minutes"
            autofocus
            @keydown.enter="submitActivate"
          />
          <AppButton @click="cancelActivate">Cancel</AppButton>
          <AppButton variant="primary" :disabled="activating" @click="submitActivate">
            {{
              statusFor(channel) === 'expired'
                ? activating
                  ? 'Activating...'
                  : 'Activate'
                : activating
                  ? 'Rotating...'
                  : 'Rotate'
            }}
          </AppButton>
        </div>
        <p v-if="activatingId === channel.ID" class="hint">
          Channel stays open for {{ activateTtlMinutes }} minutes before it needs reactivating.
        </p>
        <p v-if="activatingId === channel.ID && activateError" class="error-text">
          {{ activateError }}
        </p>
      </div>
    </div>
  </div>
</template>

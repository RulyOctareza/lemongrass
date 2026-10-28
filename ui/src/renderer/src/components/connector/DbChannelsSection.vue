<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type { VaultChannel } from '../../../../preload/types'
import AppButton from '../AppButton.vue'
import AppMultiPicker from '../AppMultiPicker.vue'

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

// Mirrors the operation strings dbgate.AllowStatement recognizes (dbgate/bouncer.go).
const operationOptions = [
  'select',
  'explain',
  'show',
  'insert',
  'update',
  'delete',
  'performance'
] as const

const showCreateForm = ref(false)
const newChannelName = ref('')
const newDbName = ref('')
const newTables = ref<string[]>([])
const newOperations = ref<string[]>([])
const newTtlMinutes = ref(10)
const creating = ref(false)
const createError = ref('')
const copiedFrom = ref('')
const rootEl = ref<HTMLElement | null>(null)
// Tables carried over by Copy, applied when the connection watcher fires so it does not clear them.
let copiedTables: string[] | null = null

const availableTables = ref<string[]>([])
const tablesLoading = ref(false)
const tablesError = ref('')

async function loadTablesFor(dbName: string): Promise<void> {
  availableTables.value = []
  tablesError.value = ''
  if (!dbName || !vaultUnlocked.value) return
  tablesLoading.value = true
  try {
    availableTables.value = await window.api.vault.listTables(sessionPassphrase.value, dbName)
  } catch (err) {
    tablesError.value = describeError(err)
  } finally {
    tablesLoading.value = false
  }
}

watch(newDbName, (dbName) => {
  newTables.value = copiedTables ?? []
  copiedTables = null
  loadTablesFor(dbName)
})

const tableGroups = computed(() => [
  { items: availableTables.value.map((t) => ({ value: t, label: t })) }
])

const tablesSummaryLabel = computed(() => {
  if (tablesLoading.value) return 'Loading tables...'
  if (newTables.value.length === 0) return 'Select tables'
  if (newTables.value.length === 1) return newTables.value[0]
  return `${newTables.value.length} tables selected`
})

function removeSelectedTable(table: string): void {
  newTables.value = newTables.value.filter((t) => t !== table)
}

function openCreateForm(): void {
  copiedFrom.value = ''
  newChannelName.value = ''
  newDbName.value = connections.value[0] ?? ''
  newTables.value = []
  newOperations.value = []
  newTtlMinutes.value = 10
  createError.value = ''
  showCreateForm.value = true
  loadTablesFor(newDbName.value)
}

function openCopyForm(channel: VaultChannel): void {
  const dbName = connections.value.includes(channel.DBName) ? channel.DBName : ''
  const connectionChanged = dbName !== newDbName.value
  copiedFrom.value = channel.Name || 'channel'
  newChannelName.value = `${copiedFrom.value} copy`
  newTables.value = [...(channel.Scope.Tables ?? [])]
  newOperations.value = [...(channel.Scope.Operations ?? [])]
  newTtlMinutes.value = 10
  createError.value = ''
  copiedTables = connectionChanged ? [...newTables.value] : null
  newDbName.value = dbName
  showCreateForm.value = true
  if (!connectionChanged) loadTablesFor(dbName)
  nextTick(() => rootEl.value?.closest('.vault-main')?.scrollTo({ top: 0, behavior: 'smooth' }))
}

function cancelCreate(): void {
  showCreateForm.value = false
}

async function submitCreate(): Promise<void> {
  if (
    !newChannelName.value.trim() ||
    !newDbName.value.trim() ||
    !vaultUnlocked.value ||
    creating.value
  )
    return
  creating.value = true
  createError.value = ''
  try {
    const { channel, shortId } = await window.api.vault.create(
      sessionPassphrase.value,
      newChannelName.value.trim(),
      newDbName.value.trim(),
      // IPC structured clone cannot clone a Vue reactive Proxy, so the arrays are spread first.
      { Tables: [...newTables.value], Operations: [...newOperations.value] },
      Math.round(newTtlMinutes.value * 60)
    )
    shortIdByChannel.value[channel.ID] = shortId
    showCreateForm.value = false
    await refresh()
  } catch (err) {
    createError.value = describeError(err)
  } finally {
    creating.value = false
  }
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

    <div v-if="showCreateForm" class="inline-section">
      <h4 class="inline-section-title">
        {{ copiedFrom ? `New channel from ${copiedFrom}` : 'New channel' }}
      </h4>

      <div class="form-columns">
        <div class="form-column">
          <div class="field-block">
            <label class="field-label" for="new-channel-name">Name</label>
            <input
              id="new-channel-name"
              v-model="newChannelName"
              type="text"
              placeholder="e.g. debug X"
            />
          </div>
          <div class="field-block">
            <label class="field-label" for="new-channel-db">Connection</label>
            <select id="new-channel-db" v-model="newDbName">
              <option v-for="name in connections" :key="name" :value="name">{{ name }}</option>
            </select>
          </div>
          <div class="field-block">
            <label class="field-label" for="new-channel-ttl">TTL</label>
            <input
              id="new-channel-ttl"
              v-model.number="newTtlMinutes"
              type="number"
              min="1"
              @keydown.enter="submitCreate"
            />
            <p class="hint">
              Channel stays open for {{ newTtlMinutes }} minutes before it needs reactivating.
            </p>
          </div>
        </div>
        <div class="form-column">
          <div class="field-block">
            <label class="field-label">Tables</label>
            <p v-if="!newDbName" class="hint">Choose a connection above first.</p>
            <p v-else-if="tablesError" class="error-text">{{ tablesError }}</p>
            <p v-else-if="!tablesLoading && availableTables.length === 0" class="hint">
              No tables found in this database.
            </p>
            <AppMultiPicker
              v-else
              v-model="newTables"
              :groups="tableGroups"
              :summary="tablesSummaryLabel"
              filter-placeholder="Filter tables..."
              :disabled="tablesLoading"
            >
              <div v-if="newTables.length > 0" class="chip-row">
                <span v-for="table in newTables" :key="table" class="chip">
                  {{ table }}
                  <button
                    type="button"
                    class="chip-remove"
                    title="Remove"
                    @click="removeSelectedTable(table)"
                  >
                    ×
                  </button>
                </span>
              </div>
            </AppMultiPicker>
          </div>
          <div class="field-block">
            <label class="field-label">Operations</label>
            <div class="checkbox-grid">
              <label v-for="op in operationOptions" :key="op" class="checkbox-option">
                <input v-model="newOperations" type="checkbox" :value="op" />
                {{ op }}
              </label>
            </div>
            <p v-if="newOperations.includes('performance')" class="hint">
              Performance lets this channel read server-wide performance views (MySQL
              performance_schema and sys, Postgres pg_stat views). They include activity from every
              database on the server. Columns holding raw statement text are never returned.
            </p>
          </div>
        </div>
      </div>

      <p v-if="createError" class="error-text">{{ createError }}</p>
      <div class="create-actions">
        <AppButton @click="cancelCreate">Cancel</AppButton>
        <AppButton
          variant="primary"
          :disabled="!newChannelName.trim() || !newDbName.trim() || creating"
          @click="submitCreate"
        >
          {{ creating ? 'Creating...' : 'Create' }}
        </AppButton>
      </div>
    </div>

    <p v-if="listError" class="error-text">{{ listError }}</p>
    <p v-else-if="loading" class="empty">Loading...</p>
    <p v-else-if="channels.length === 0" class="empty">No channels yet.</p>

    <div v-else class="channel-list">
      <div v-for="channel in channels" :key="channel.ID" class="channel-row channel-row--stacked">
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
          <input
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

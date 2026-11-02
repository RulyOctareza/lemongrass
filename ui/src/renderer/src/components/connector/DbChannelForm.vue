<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The db channel creation form, used by the Channels section and by a db ask. Its styles come from
// vault.css, so it must render inside a .vault-tab element. A parent reopens it with fresh values by
// remounting it under a new key.
import { computed, onMounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type { VaultChannelWithShortId } from '../../../../preload/types'
import AppButton from '../AppButton.vue'
import AppSwitcher from '../AppSwitcher.vue'
import AppMultiPicker from '../AppMultiPicker.vue'
import AppInput from '../AppInput.vue'

export interface DbChannelSeed {
  name: string
  dbName: string
  tables: string[]
  operations: string[]
}

const props = defineProps<{
  connections: string[]
  title: string
  seed?: DbChannelSeed
  // A db ask's tables: kept selected across connection changes, and Create waits until the chosen connection has them all.
  requestedTables?: string[]
  // Set when the channel answers a model's ask; the main process answers the ask with the short id.
  askId?: string
}>()

const emit = defineEmits<{
  created: [result: VaultChannelWithShortId]
  cancel: []
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

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

const newChannelName = ref(props.seed?.name ?? '')
const newDbName = ref(props.seed?.dbName ?? props.connections[0] ?? '')
const newTables = ref<string[]>([...(props.seed?.tables ?? props.requestedTables ?? [])])
const newOperations = ref<string[]>([...(props.seed?.operations ?? [])])
const newTtlMinutes = ref(10)
const creating = ref(false)
const createError = ref('')

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

// Registered after the seed is applied, so only a change the human makes resets the tables.
watch(newDbName, (dbName) => {
  newTables.value = [...(props.requestedTables ?? [])]
  loadTablesFor(dbName)
})

onMounted(() => loadTablesFor(newDbName.value))

const tableGroups = computed(() => [
  { items: availableTables.value.map((t) => ({ value: t, label: t })) }
])

const tablesSummaryLabel = computed(() => {
  if (tablesLoading.value) return 'Loading tables...'
  if (newTables.value.length === 0) return 'Select tables'
  if (newTables.value.length === 1) return newTables.value[0]
  return `${newTables.value.length} tables selected`
})

const missingTables = computed(() => {
  if (!props.requestedTables || tablesLoading.value || tablesError.value || !newDbName.value)
    return []
  const present = new Set(availableTables.value)
  return props.requestedTables.filter((t) => !present.has(t))
})

function removeSelectedTable(table: string): void {
  newTables.value = newTables.value.filter((t) => t !== table)
}

const canCreate = computed(
  () =>
    !!newChannelName.value.trim() &&
    !!newDbName.value.trim() &&
    vaultUnlocked.value &&
    !creating.value &&
    missingTables.value.length === 0
)

async function submitCreate(): Promise<void> {
  if (!canCreate.value) return
  creating.value = true
  createError.value = ''
  try {
    const result = await window.api.vault.create(
      sessionPassphrase.value,
      newChannelName.value.trim(),
      newDbName.value.trim(),
      // IPC structured clone cannot clone a Vue reactive Proxy, so the arrays are spread first.
      { Tables: [...newTables.value], Operations: [...newOperations.value] },
      Math.round(newTtlMinutes.value * 60),
      props.askId
    )
    emit('created', result)
  } catch (err) {
    createError.value = describeError(err)
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="inline-section">
    <h4 class="inline-section-title">{{ title }}</h4>

    <div class="form-columns">
      <div class="form-column">
        <div class="field-block">
          <label class="field-label" for="new-channel-name">Name</label>
          <AppInput
            id="new-channel-name"
            v-model="newChannelName"
            type="text"
            placeholder="e.g. debug X"
          />
        </div>
        <div class="field-block">
          <span class="field-label">Connection</span>
          <AppSwitcher
            :items="connections.map((name) => ({ id: name, label: name }))"
            :active-id="newDbName"
            placeholder="Pick a connection"
            empty-text="No connections."
            framed
            searchable
            search-placeholder="Search connections"
            @select="newDbName = $event"
          />
        </div>
        <div class="field-block">
          <label class="field-label" for="new-channel-ttl">TTL</label>
          <AppInput
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
          <p v-if="missingTables.length > 0" class="error-text">
            This connection has no table named {{ missingTables.join(', ') }}. Choose another
            connection.
          </p>
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
      <AppButton @click="emit('cancel')">Cancel</AppButton>
      <AppButton variant="primary" :disabled="!canCreate" @click="submitCreate">
        {{ creating ? 'Creating...' : 'Create' }}
      </AppButton>
    </div>
  </div>
</template>

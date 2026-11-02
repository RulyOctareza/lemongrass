<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The grant view of a waiting db ask: the human picks a connection and a TTL, and the tables and
// operations the model declared come prefilled. The daemon holds the same rules on create (granted
// tables and operations a subset of the declared ones, every granted table in the connection), so
// the checks here only spare the human a refused Approve. Renders inside a .vault-tab element.
import { computed, ref, watch } from 'vue'
import type { Ask, AskDeclineExtra, VaultChannelWithShortId } from '../../../../preload/types'
import { describeError, vaultSession } from '../../vaultSession'
import AppButton from '../AppButton.vue'
import AppCheckbox from '../AppCheckbox.vue'
import AppInfoPopover from '../AppInfoPopover.vue'
import AppInput from '../AppInput.vue'
import AppSwitcher from '../AppSwitcher.vue'

const props = defineProps<{
  ask: Ask
  connections: string[]
}>()

const emit = defineEmits<{
  created: [result: VaultChannelWithShortId]
  decline: []
}>()

type CapsuleState = 'unchecked' | 'checking' | 'found' | 'missing'

// The human's picks: a capsule reads red until its table is found, and the operation boxes open for
// unchecking only once every remaining table is found. One value each, so either is easy to change.
const UNCHECKED_TONE: 'missing' | 'checking' = 'missing'
const OPERATIONS_EDITABLE_AFTER_CHECK = true

const requested = computed(() => props.ask.tables ?? [])
const declared = computed(() => props.ask.operations ?? [])

const dbName = ref('')
const ttlMinutes = ref(10)
const removed = ref<string[]>([])
const operations = ref<string[]>([...declared.value])

// The picked connection's tables by lower-cased name, since the daemon compares without case and the
// grant should carry the connection's own spelling.
const tablesByLower = ref(new Map<string, string>())
const tablesLoading = ref(false)
const tablesError = ref('')
let loadSeq = 0

async function pickConnection(name: string): Promise<void> {
  dbName.value = name
  // A removal belongs to the connection it was missing from.
  removed.value = []
  tablesByLower.value = new Map()
  tablesError.value = ''
  const seq = ++loadSeq
  tablesLoading.value = true
  try {
    const list = await window.api.vault.listTables(vaultSession.passphrase, name)
    if (seq !== loadSeq) return
    tablesByLower.value = new Map(list.map((t) => [t.toLowerCase(), t]))
  } catch (err) {
    if (seq === loadSeq) tablesError.value = describeError(err)
  } finally {
    if (seq === loadSeq) tablesLoading.value = false
  }
}

const checked = computed(() => !!dbName.value && !tablesLoading.value && !tablesError.value)

function capsuleState(table: string): CapsuleState {
  if (!dbName.value || tablesError.value) return 'unchecked'
  if (tablesLoading.value) return 'checking'
  return tablesByLower.value.has(table.toLowerCase()) ? 'found' : 'missing'
}

function capsuleTone(table: string): string {
  const state = capsuleState(table)
  return state === 'unchecked' ? UNCHECKED_TONE : state
}

function capsuleHint(table: string): string {
  switch (capsuleState(table)) {
    case 'unchecked':
      return `Pick a connection to check table ${table}.`
    case 'checking':
      return `Checking table ${table} in connection ${dbName.value}.`
    case 'found':
      return `Table ${table} exists in connection ${dbName.value}.`
    default:
      return `Table ${table} does not exist in connection ${dbName.value}.`
  }
}

const remaining = computed(() => requested.value.filter((t) => !removed.value.includes(t)))
const missing = computed(() =>
  checked.value ? remaining.value.filter((t) => capsuleState(t) === 'missing') : []
)
const allFound = computed(
  () => checked.value && remaining.value.length > 0 && missing.value.length === 0
)

const operationsLocked = computed(() => !OPERATIONS_EDITABLE_AFTER_CHECK || !allFound.value)

// Locked boxes read as declared, so a check that fails again puts every declared operation back.
watch(operationsLocked, (locked) => {
  if (locked) operations.value = [...declared.value]
})

function toggleOperation(op: string): void {
  if (operationsLocked.value) return
  const i = operations.value.indexOf(op)
  if (i === -1) operations.value.push(op)
  else operations.value.splice(i, 1)
}

function removeTable(table: string): void {
  if (capsuleState(table) !== 'missing') return
  removed.value = [...removed.value, table]
}

const creating = ref(false)
const createError = ref('')

const holdReason = computed(() => {
  if (declared.value.length === 0)
    return 'The ask declares no operations, so it can only be declined.'
  if (!dbName.value) return 'Pick a connection to check the tables.'
  if (tablesLoading.value) return `Checking the tables in connection ${dbName.value}.`
  if (tablesError.value) return ''
  if (remaining.value.length === 0)
    return 'Every table is removed, so the ask can only be declined.'
  if (missing.value.length > 0)
    return `Connection ${dbName.value} has no table ${missing.value.join(', ')}. Remove it or pick another connection to approve.`
  if (operations.value.length === 0) return 'Check at least one operation to approve.'
  return ''
})

const canApprove = computed(
  () =>
    allFound.value &&
    declared.value.length > 0 &&
    operations.value.length > 0 &&
    ttlMinutes.value > 0 &&
    vaultSession.unlocked &&
    !creating.value
)

async function approve(): Promise<void> {
  if (!canApprove.value) return
  creating.value = true
  createError.value = ''
  try {
    const tables = remaining.value.map((t) => tablesByLower.value.get(t.toLowerCase()) ?? t)
    const result = await window.api.vault.create(
      vaultSession.passphrase,
      `ask ${props.ask.id}`,
      dbName.value,
      { Tables: tables, Operations: [...operations.value] },
      Math.round(ttlMinutes.value * 60),
      props.ask.id
    )
    emit('created', result)
  } catch (err) {
    createError.value = describeError(err)
  } finally {
    creating.value = false
  }
}

// Read by the modal's decline step, so a decline carries the tables removed and where they were missing.
function declineExtra(): AskDeclineExtra {
  return {
    dbName: removed.value.length > 0 ? dbName.value : undefined,
    removedTables: [...removed.value]
  }
}

defineExpose({ declineExtra })

// One hint card serves every capsule: it follows whichever capsule is hovered or focused.
const hintTable = ref<string | null>(null)
const hintAnchor = ref<HTMLElement | null>(null)
const hintId = computed(() => `ask-table-hint-${props.ask.id}`)

function showHint(table: string, event: Event): void {
  hintTable.value = table
  hintAnchor.value = event.currentTarget as HTMLElement
}

function hideHint(event: Event): void {
  const next = (event as FocusEvent).relatedTarget as Node | null
  if (next && hintAnchor.value?.contains(next)) return
  hintTable.value = null
  hintAnchor.value = null
}

watch(remaining, (tables) => {
  if (hintTable.value && !tables.includes(hintTable.value)) {
    hintTable.value = null
    hintAnchor.value = null
  }
})
</script>

<template>
  <div class="inline-section">
    <div class="form-columns">
      <div class="form-column">
        <div class="field-block">
          <span class="field-label">Connection</span>
          <AppSwitcher
            :items="connections.map((name) => ({ id: name, label: name }))"
            :active-id="dbName"
            placeholder="Pick a connection"
            empty-text="No connections."
            framed
            searchable
            search-placeholder="Search connections"
            @select="pickConnection"
          />
        </div>
        <div class="field-block">
          <label class="field-label" :for="`ask-ttl-${ask.id}`">TTL</label>
          <AppInput
            :id="`ask-ttl-${ask.id}`"
            v-model.number="ttlMinutes"
            type="number"
            min="1"
            @keydown.enter="approve"
          />
          <p class="hint">
            The channel stays open for {{ ttlMinutes }} minutes before it needs reactivating.
          </p>
        </div>
      </div>

      <div class="form-column">
        <div class="field-block">
          <span class="field-label">Tables</span>
          <div class="capsule-row">
            <span
              v-for="table in remaining"
              :key="table"
              class="capsule"
              :class="`capsule--${capsuleTone(table)}`"
              tabindex="0"
              :aria-describedby="hintTable === table ? hintId : undefined"
              @mouseenter="showHint(table, $event)"
              @mouseleave="hideHint"
              @focusin="showHint(table, $event)"
              @focusout="hideHint"
            >
              {{ table }}
              <button
                v-if="capsuleState(table) === 'missing'"
                type="button"
                class="capsule-remove"
                :aria-label="`Remove table ${table}`"
                @click="removeTable(table)"
              >
                ×
              </button>
            </span>
          </div>
          <AppInfoPopover :id="hintId" :anchor="hintAnchor">
            {{ hintTable ? capsuleHint(hintTable) : '' }}
          </AppInfoPopover>
          <p v-if="tablesError" class="error-text">{{ tablesError }}</p>
          <p v-if="removed.length > 0" class="hint">
            Removed {{ removed.join(', ') }}. The requester is told
            {{ removed.length === 1 ? 'it does' : 'they do' }} not exist in connection {{ dbName }}.
            Picking another connection puts {{ removed.length === 1 ? 'it' : 'them' }} back.
          </p>
        </div>

        <div class="field-block">
          <span class="field-label">Operations</span>
          <div class="checkbox-grid">
            <AppCheckbox
              v-for="op in declared"
              :key="op"
              class="op-box"
              :checked="operations.includes(op)"
              :disabled="operationsLocked"
              @click="toggleOperation(op)"
            >
              {{ op }}
            </AppCheckbox>
          </div>
          <p
            v-if="OPERATIONS_EDITABLE_AFTER_CHECK && operationsLocked && declared.length > 0"
            class="hint"
          >
            The operations stay as the requester declared them until every table is found.
          </p>
          <p v-if="declared.includes('performance')" class="hint">
            Performance opens the server-wide performance views (MySQL performance_schema and sys,
            Postgres pg_stat views), with activity from every database on the server. Columns
            holding raw statement text are never returned.
          </p>
        </div>
      </div>
    </div>

    <p v-if="createError" class="error-text">{{ createError }}</p>
    <p v-else-if="holdReason" class="hint">{{ holdReason }}</p>
    <div class="create-actions">
      <AppButton :disabled="creating" @click="emit('decline')">Decline</AppButton>
      <AppButton variant="primary" :disabled="!canApprove" @click="approve">
        {{ creating ? 'Approving...' : 'Approve' }}
      </AppButton>
    </div>
  </div>
</template>

<style scoped>
.capsule-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.capsule {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  outline: none;
  transition:
    background var(--duration-fast) var(--ease-default),
    color var(--duration-fast) var(--ease-default);
}

.capsule:focus-visible {
  outline: 1px solid var(--color-amber);
  outline-offset: 1px;
}

.capsule--missing {
  background: var(--color-error-muted);
  color: var(--color-error);
}

.capsule--checking {
  background: var(--color-surface-3);
  color: var(--color-fg-secondary);
}

.capsule--found {
  background: var(--color-jade-muted);
  color: var(--color-jade);
}

.capsule-remove {
  padding: 0;
  background: transparent;
  border: none;
  color: inherit;
  font-size: var(--text-sm);
  line-height: 1;
  cursor: pointer;
  opacity: 0.7;
}

.capsule-remove:hover,
.capsule-remove:focus-visible {
  opacity: 1;
}

.op-box:disabled {
  cursor: default;
  opacity: 0.7;
}
</style>

<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onMounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import AppButton from '../AppButton.vue'
import AppCheckbox from '../AppCheckbox.vue'
import AppSwitcher from '../AppSwitcher.vue'
import type { ResolvedSSHHost } from '../../../../preload/types'
import {
  composeConnectionString as composeFields,
  parseConnectionString,
  type EngineChoice
} from '../../connectionString'

const props = defineProps<{
  visible: boolean
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

const connections = ref<string[]>([])
const connectionsLoading = ref(false)
const connectionsError = ref('')
const sshHostByConnection = ref<Record<string, string>>({})
const sshHostByConnectionError = ref('')

async function refreshConnections(): Promise<void> {
  connectionsLoading.value = true
  connectionsError.value = ''
  try {
    connections.value = await window.api.vault.listConnections()
  } catch (err) {
    connectionsError.value = describeError(err)
  } finally {
    connectionsLoading.value = false
  }
  await refreshSSHHostByConnection()
}

// The alias lives inside each encrypted connection string, so reading it takes the passphrase.
async function refreshSSHHostByConnection(): Promise<void> {
  if (!vaultUnlocked.value) return
  sshHostByConnectionError.value = ''
  try {
    sshHostByConnection.value = await window.api.vault.connectionSSHHosts(sessionPassphrase.value)
  } catch (err) {
    sshHostByConnectionError.value = describeError(err)
  }
}

const rowStatus = ref<Record<string, 'verifying' | 'ok' | 'error'>>({})
const rowStatusMessage = ref<Record<string, string>>({})

async function verifyRow(name: string): Promise<void> {
  rowStatus.value[name] = 'verifying'
  try {
    await window.api.vault.testSavedConnection(sessionPassphrase.value, name)
    rowStatus.value[name] = 'ok'
    rowStatusMessage.value[name] = 'Connected'
  } catch (err) {
    rowStatus.value[name] = 'error'
    rowStatusMessage.value[name] = describeError(err)
  }
}

async function verifyAllConnections(): Promise<void> {
  if (!vaultUnlocked.value) return
  await Promise.all(connections.value.map(verifyRow))
}

async function testRow(name: string): Promise<void> {
  if (!vaultUnlocked.value) return
  await verifyRow(name)
}

function capsuleLabel(status?: 'verifying' | 'ok' | 'error'): string {
  switch (status) {
    case 'verifying':
      return 'Testing…'
    case 'ok':
      return 'Connected'
    case 'error':
      return 'Invalid'
    default:
      return 'Not tested'
  }
}

type TestStatus = 'idle' | 'testing' | 'ok' | 'error'

const defaultPorts: Record<EngineChoice, string> = {
  mysql: '3306',
  mariadb: '3306',
  postgres: '5432'
}

const showConnectionForm = ref(false)
const editingConnectionName = ref<string | null>(null)
const loadingConnectionName = ref<string | null>(null)
const preservedConnectionQuery = ref('')
const newConnectionName = ref('')
const newConnectionEngine = ref<EngineChoice>('mysql')
const newConnectionHost = ref('')
const newConnectionPort = ref(defaultPorts.mysql)
const newConnectionUser = ref('')
const newConnectionPassword = ref('')
const newConnectionDatabase = ref('')
const useSSHTunnel = ref(false)
const newConnectionSSHHost = ref('')
const sshConfigHosts = ref<string[]>([])
const sshConfigHostsError = ref('')
const resolvedSSHHost = ref<ResolvedSSHHost | null>(null)
const resolvedSSHHostError = ref('')
const resolvingSSHHost = ref(false)
const connectionFormError = ref('')
const creatingConnection = ref(false)

const newConnectionTestStatus = ref<TestStatus>('idle')
const newConnectionTestError = ref('')

// Any field the composed string is built from invalidates a previous test.
watch(
  [
    newConnectionEngine,
    newConnectionHost,
    newConnectionPort,
    newConnectionUser,
    newConnectionPassword,
    newConnectionDatabase,
    useSSHTunnel,
    newConnectionSSHHost
  ],
  () => {
    newConnectionTestStatus.value = 'idle'
    newConnectionTestError.value = ''
  }
)

function composeConnectionString(): string {
  return composeFields({
    engine: newConnectionEngine.value,
    host: newConnectionHost.value,
    port: newConnectionPort.value,
    user: newConnectionUser.value,
    password: newConnectionPassword.value,
    database: newConnectionDatabase.value,
    sshHost: useSSHTunnel.value ? newConnectionSSHHost.value : '',
    query: preservedConnectionQuery.value
  })
}

async function loadSSHConfigHosts(): Promise<void> {
  sshConfigHostsError.value = ''
  try {
    sshConfigHosts.value = await window.api.vault.listSSHHosts()
  } catch (err) {
    sshConfigHosts.value = []
    sshConfigHostsError.value = describeError(err)
  }
}

// A stored alias that is no longer in the config stays pickable and is marked, never dropped.
const sshHostItems = computed(() => {
  const items = sshConfigHosts.value.map((host) => ({ id: host, label: host }))
  const current = newConnectionSSHHost.value
  if (current && !sshConfigHosts.value.includes(current)) {
    items.unshift({ id: current, label: `${current} (not in ~/.ssh/config)` })
  }
  return items
})

let resolveSeq = 0

async function resolvePickedSSHHost(): Promise<void> {
  const seq = ++resolveSeq
  resolvedSSHHost.value = null
  resolvedSSHHostError.value = ''
  const alias = newConnectionSSHHost.value
  if (!useSSHTunnel.value || !alias) {
    resolvingSSHHost.value = false
    return
  }
  resolvingSSHHost.value = true
  try {
    const resolved = await window.api.vault.resolveSSHHost(alias)
    if (seq === resolveSeq) resolvedSSHHost.value = resolved
  } catch (err) {
    if (seq === resolveSeq) resolvedSSHHostError.value = describeError(err)
  } finally {
    if (seq === resolveSeq) resolvingSSHHost.value = false
  }
}

watch([useSSHTunnel, newConnectionSSHHost], resolvePickedSSHHost)

function describeResolvedSSHHost(host: ResolvedSSHHost): string {
  const target = `${host.user ? host.user + '@' : ''}${host.hostname}${host.port ? ':' + host.port : ''}`
  return host.proxyJump ? `${target}, through ${host.proxyJump}` : target
}

function toggleSSHTunnel(): void {
  useSSHTunnel.value = !useSSHTunnel.value
  if (useSSHTunnel.value) void loadSSHConfigHosts()
}

function openConnectionForm(): void {
  editingConnectionName.value = null
  preservedConnectionQuery.value = ''
  newConnectionName.value = ''
  newConnectionEngine.value = 'mysql'
  newConnectionHost.value = ''
  newConnectionPort.value = defaultPorts.mysql
  newConnectionUser.value = ''
  newConnectionPassword.value = ''
  newConnectionDatabase.value = ''
  useSSHTunnel.value = false
  newConnectionSSHHost.value = ''
  connectionFormError.value = ''
  newConnectionTestStatus.value = 'idle'
  newConnectionTestError.value = ''
  showConnectionForm.value = true
}

function cancelConnectionForm(): void {
  showConnectionForm.value = false
}

async function openConnectionEdit(name: string): Promise<void> {
  if (!vaultUnlocked.value || loadingConnectionName.value) return
  loadingConnectionName.value = name
  connectionsError.value = ''
  try {
    const stored = await window.api.vault.getConnection(sessionPassphrase.value, name)
    const parsed = parseConnectionString(stored)
    editingConnectionName.value = name
    preservedConnectionQuery.value = parsed.query
    newConnectionName.value = name
    newConnectionEngine.value = parsed.engine
    newConnectionHost.value = parsed.host
    newConnectionPort.value = parsed.port
    newConnectionUser.value = parsed.user
    newConnectionPassword.value = parsed.password
    newConnectionDatabase.value = parsed.database
    useSSHTunnel.value = parsed.sshHost !== ''
    newConnectionSSHHost.value = parsed.sshHost
    if (useSSHTunnel.value) void loadSSHConfigHosts()
    connectionFormError.value = ''
    newConnectionTestStatus.value = 'idle'
    newConnectionTestError.value = ''
    showConnectionForm.value = true
  } catch (err) {
    connectionsError.value = describeError(err)
  } finally {
    loadingConnectionName.value = null
  }
}

// Only fills the port when it is empty or still at an engine default.
function onEngineChange(): void {
  if (!newConnectionPort.value || Object.values(defaultPorts).includes(newConnectionPort.value)) {
    newConnectionPort.value = defaultPorts[newConnectionEngine.value]
  }
}

const canTestNewConnection = computed(
  () =>
    newConnectionHost.value.trim().length > 0 &&
    (!useSSHTunnel.value || newConnectionSSHHost.value !== '') &&
    newConnectionTestStatus.value !== 'testing'
)

async function testNewConnection(): Promise<void> {
  if (!canTestNewConnection.value) return
  newConnectionTestStatus.value = 'testing'
  newConnectionTestError.value = ''
  try {
    await window.api.vault.testConnection(composeConnectionString())
    newConnectionTestStatus.value = 'ok'
  } catch (err) {
    newConnectionTestStatus.value = 'error'
    newConnectionTestError.value = describeError(err)
  }
}

async function submitConnection(): Promise<void> {
  if (
    !newConnectionName.value.trim() ||
    !vaultUnlocked.value ||
    newConnectionTestStatus.value !== 'ok' ||
    creatingConnection.value
  )
    return
  creatingConnection.value = true
  connectionFormError.value = ''
  try {
    if (editingConnectionName.value !== null) {
      await window.api.vault.updateConnection(
        sessionPassphrase.value,
        editingConnectionName.value,
        composeConnectionString()
      )
    } else {
      await window.api.vault.putCredential(
        sessionPassphrase.value,
        newConnectionName.value.trim(),
        composeConnectionString()
      )
    }
    showConnectionForm.value = false
    await refreshConnections()
    await verifyAllConnections()
  } catch (err) {
    connectionFormError.value = describeError(err)
  } finally {
    creatingConnection.value = false
  }
}

const deletingConnectionName = ref<string | null>(null)

async function deleteConnection(name: string): Promise<void> {
  deletingConnectionName.value = name
  try {
    await window.api.vault.deleteConnection(name)
    delete rowStatus.value[name]
    delete rowStatusMessage.value[name]
    await refreshConnections()
  } catch (err) {
    connectionsError.value = describeError(err)
  } finally {
    deletingConnectionName.value = null
  }
}

onMounted(() => {
  refreshConnections().then(verifyAllConnections)
})

watch(
  () => props.visible,
  (visible) => {
    if (visible) refreshConnections()
  }
)
</script>

<template>
  <div class="section">
    <div class="section-toolbar">
      <p class="hint">Saved database credentials. Channels are created from a connection.</p>
      <AppButton
        v-if="!showConnectionForm"
        variant="primary"
        class="new-channel-button"
        @click="openConnectionForm"
      >
        + New connection
      </AppButton>
    </div>

    <div v-if="showConnectionForm" class="inline-section">
      <h4 class="inline-section-title">
        {{ editingConnectionName !== null ? 'Edit connection' : 'New connection' }}
      </h4>

      <div>
        <label class="field-label" for="conn-name">Name</label>
        <input
          id="conn-name"
          v-model="newConnectionName"
          type="text"
          placeholder="e.g. staging-db"
          :disabled="editingConnectionName !== null"
          autofocus
        />
      </div>

      <div class="ssh-tunnel">
        <AppCheckbox :checked="useSSHTunnel" @click="toggleSSHTunnel">Use SSH tunnel</AppCheckbox>
        <template v-if="useSSHTunnel">
          <label class="field-label">SSH host</label>
          <AppSwitcher
            framed
            :items="sshHostItems"
            :active-id="newConnectionSSHHost || null"
            placeholder="Pick a host from ~/.ssh/config"
            empty-text="No Host entries in ~/.ssh/config."
            @select="(host) => (newConnectionSSHHost = host)"
          />
          <p v-if="sshConfigHostsError" class="error-text">{{ sshConfigHostsError }}</p>
          <p v-if="resolvingSSHHost" class="hint">Reading the host from your ssh config...</p>
          <p v-else-if="resolvedSSHHostError" class="error-text">{{ resolvedSSHHostError }}</p>
          <p v-else-if="resolvedSSHHost" class="hint">
            Tunnel goes to {{ describeResolvedSSHHost(resolvedSSHHost) }}
          </p>
        </template>
      </div>

      <div class="field-grid">
        <div>
          <label class="field-label" for="conn-engine">Engine</label>
          <select
            id="conn-engine"
            v-model="newConnectionEngine"
            :disabled="editingConnectionName !== null"
            @change="onEngineChange"
          >
            <option value="mysql">MySQL</option>
            <option value="mariadb">MariaDB</option>
            <option value="postgres">Postgres</option>
          </select>
        </div>
        <div>
          <label class="field-label" for="conn-host">
            {{ useSSHTunnel ? 'Database host, from the SSH host' : 'Host' }}
          </label>
          <input id="conn-host" v-model="newConnectionHost" type="text" placeholder="localhost" />
        </div>
        <div>
          <label class="field-label" for="conn-port">Port</label>
          <input id="conn-port" v-model="newConnectionPort" type="text" placeholder="3306" />
        </div>
        <div>
          <label class="field-label" for="conn-database">Database</label>
          <input
            id="conn-database"
            v-model="newConnectionDatabase"
            type="text"
            placeholder="db name"
          />
        </div>
        <div>
          <label class="field-label" for="conn-user">Username</label>
          <input id="conn-user" v-model="newConnectionUser" type="text" placeholder="db user" />
        </div>
        <div>
          <label class="field-label" for="conn-password">Password</label>
          <input
            id="conn-password"
            v-model="newConnectionPassword"
            type="password"
            placeholder="db password"
          />
        </div>
      </div>

      <div class="test-row">
        <AppButton :disabled="!canTestNewConnection" @click="testNewConnection">
          {{ newConnectionTestStatus === 'testing' ? 'Testing...' : 'Test connection' }}
        </AppButton>
        <span
          class="test-status"
          :class="{
            'test-status-ok': newConnectionTestStatus === 'ok',
            'test-status-error': newConnectionTestStatus === 'error'
          }"
        >
          <template v-if="newConnectionTestStatus === 'ok'">✓ Connected</template>
          <template v-else-if="newConnectionTestStatus === 'error'">
            ✗ {{ newConnectionTestError }}
          </template>
          <template v-else-if="newConnectionTestStatus === 'testing'">Testing...</template>
          <template v-else>Not tested yet</template>
        </span>
      </div>

      <p v-if="!vaultUnlocked" class="hint">Unlock the vault above before saving.</p>

      <p v-if="connectionFormError" class="error-text">{{ connectionFormError }}</p>

      <div class="create-actions">
        <AppButton @click="cancelConnectionForm">Cancel</AppButton>
        <AppButton
          variant="primary"
          :disabled="
            !newConnectionName.trim() ||
            !vaultUnlocked ||
            newConnectionTestStatus !== 'ok' ||
            creatingConnection
          "
          :title="
            !vaultUnlocked
              ? 'Unlock the vault first'
              : newConnectionTestStatus !== 'ok'
                ? 'Test the connection before saving'
                : ''
          "
          @click="submitConnection"
        >
          {{ creatingConnection ? 'Saving...' : 'Save' }}
        </AppButton>
      </div>
    </div>

    <p v-if="sshHostByConnectionError" class="error-text">{{ sshHostByConnectionError }}</p>
    <p v-if="connectionsError" class="error-text">{{ connectionsError }}</p>
    <p v-else-if="connectionsLoading" class="empty">Loading...</p>
    <p v-else-if="connections.length === 0" class="empty">No connections yet.</p>

    <div v-else class="channel-list">
      <div v-for="name in connections" :key="name" class="channel-row">
        <div class="channel-info">
          <span class="channel-db">{{ name }}</span>
          <span v-if="sshHostByConnection[name]" class="value-capsule">
            via {{ sshHostByConnection[name] }}
          </span>
          <span
            class="status-capsule"
            :class="'status-' + (rowStatus[name] ?? 'idle')"
            :title="rowStatusMessage[name] || ''"
          >
            {{ capsuleLabel(rowStatus[name]) }}
          </span>
        </div>
        <div class="channel-actions">
          <AppButton
            :disabled="!vaultUnlocked || rowStatus[name] === 'verifying'"
            :title="!vaultUnlocked ? 'Unlock the vault first' : ''"
            @click="testRow(name)"
          >
            {{ rowStatus[name] === 'verifying' ? 'Testing...' : 'Test' }}
          </AppButton>
          <AppButton
            :disabled="!vaultUnlocked || loadingConnectionName === name"
            :title="!vaultUnlocked ? 'Unlock the vault first' : ''"
            @click="openConnectionEdit(name)"
          >
            {{ loadingConnectionName === name ? 'Loading...' : 'Edit' }}
          </AppButton>
          <AppButton
            variant="danger"
            :disabled="deletingConnectionName === name"
            @click="deleteConnection(name)"
          >
            {{ deletingConnectionName === name ? 'Deleting...' : 'Delete' }}
          </AppButton>
        </div>
      </div>
    </div>
  </div>
</template>

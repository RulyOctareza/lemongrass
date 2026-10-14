<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppModal from './AppModal.vue'
import type { NodeGrantView, NodeIssued, NodeState } from '../../../preload/types'

const emit = defineEmits<{
  close: []
}>()

const state = ref<NodeState | null>(null)
const error = ref('')
const busy = ref(false)

const tunnel = reactive({
  provider: 'ngrok' as 'ngrok' | 'local',
  authtoken: '',
  domain: '',
  localPort: 0
})
const hours = ref(8)

const label = ref('')
const days = ref(30)
const picked = reactive<Record<string, { books: string[]; tasks: string[] }>>({})
const expanded = ref<string | null>(null)
const issued = ref<(NodeIssued & { label: string }) | null>(null)

function apply(next: NodeState): void {
  state.value = next
  tunnel.provider = next.tunnel.provider
  tunnel.domain = next.tunnel.domain
  tunnel.localPort = next.tunnel.localPort
  for (const p of next.projects) picked[p.projectId] ??= { books: [], tasks: [] }
}

async function run(fn: () => Promise<NodeState | void>): Promise<void> {
  busy.value = true
  error.value = ''
  try {
    const next = await fn()
    if (next) apply(next)
  } catch (err) {
    error.value =
      err instanceof Error
        ? err.message.replace(/^Error invoking remote method '[^']+': (Error: )?/, '')
        : String(err)
  } finally {
    busy.value = false
  }
}

onMounted(() => run(() => window.api.node.state()))

function saveTunnel(): Promise<void> {
  return run(() =>
    window.api.node.saveTunnel(
      tunnel.provider,
      tunnel.authtoken ? tunnel.authtoken : null,
      tunnel.domain,
      tunnel.localPort
    )
  ).then(() => {
    tunnel.authtoken = ''
  })
}

const openTunnel = (): Promise<void> => run(() => window.api.node.openTunnel(hours.value))
const closeTunnel = (): Promise<void> => run(() => window.api.node.closeTunnel())
const revoke = (id: string): Promise<void> => run(() => window.api.node.revoke(id))
const remove = (id: string): Promise<void> => run(() => window.api.node.remove(id))

const status = computed(() => state.value?.status)
const nodeURL = computed(() => status.value?.url ?? '')

const pickedCount = computed(() =>
  Object.values(picked).reduce((n, p) => n + p.books.length + p.tasks.length, 0)
)

function issue(): Promise<void> {
  const shares = Object.entries(picked)
    .filter(([, p]) => p.books.length + p.tasks.length > 0)
    .map(([projectId, p]) => ({ projectId, books: [...p.books], tasks: [...p.tasks] }))
  return run(async () => {
    const result = await window.api.node.issue({ label: label.value, days: days.value, shares })
    issued.value = { ...result, label: label.value }
    label.value = ''
    for (const p of Object.values(picked)) {
      p.books = []
      p.tasks = []
    }
    return window.api.node.state()
  })
}

function serverName(text: string): string {
  return (
    'lgrass-' +
    (text
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '') || 'node')
  )
}

const mcpURL = computed(() => (nodeURL.value || 'NODE_URL') + '/mcp')

const claudeCommand = computed(() =>
  issued.value
    ? `claude mcp add --transport http ${serverName(issued.value.label)} ${mcpURL.value} --header "Authorization: Bearer ${issued.value.token}"`
    : ''
)

const mcpJSON = computed(() =>
  issued.value
    ? JSON.stringify(
        {
          mcpServers: {
            [serverName(issued.value.label)]: {
              type: 'http',
              url: mcpURL.value,
              headers: { Authorization: `Bearer ${issued.value.token}` }
            }
          }
        },
        null,
        2
      )
    : ''
)

function copy(text: string): void {
  void navigator.clipboard.writeText(text)
}

function shareSummary(g: NodeGrantView): string {
  return g.shares
    .map((s) => {
      const parts: string[] = []
      if (s.books.length) parts.push(`${s.books.length} book${s.books.length === 1 ? '' : 's'}`)
      if (s.tasks.length) parts.push(`${s.tasks.length} task${s.tasks.length === 1 ? '' : 's'}`)
      return `${s.alias}: ${parts.join(', ')}`
    })
    .join('; ')
}

function grantState(g: NodeGrantView): string {
  if (g.revoked) return 'Revoked'
  if (new Date(g.expiresAt).getTime() <= Date.now()) return 'Expired'
  return 'Active'
}

function when(iso?: string): string {
  if (!iso) return 'never'
  return new Date(iso).toLocaleString()
}
</script>

<template>
  <AppModal scope="app" size="lg" title="Node" @close="emit('close')">
    <div v-if="state" class="node">
      <p class="note">
        Temporary screen. Shares the books and tasks you pick with other people's agents over MCP,
        read only.
      </p>
      <p v-if="error" class="error">{{ error }}</p>

      <section>
        <h4>Tunnel</h4>
        <p v-if="status?.daemonDown" class="error">
          The node daemon is not running. Restart the app to start it.
        </p>
        <p v-else-if="status?.running" class="status on">
          Open until {{ when(status.until) }} at
          <code>{{ nodeURL }}</code>
          <button class="ghost-button" @click="copy(nodeURL)">Copy</button>
        </p>
        <p v-else class="status">Closed.</p>
        <p v-if="status?.error" class="error">{{ status.error }}</p>

        <div class="row">
          <label>
            Provider
            <select v-model="tunnel.provider">
              <option value="ngrok">ngrok</option>
              <option value="local">This machine only</option>
            </select>
          </label>
          <template v-if="tunnel.provider === 'ngrok'">
            <label>
              Authtoken
              <input
                v-model="tunnel.authtoken"
                type="password"
                :placeholder="
                  state.tunnel.authtokenSet
                    ? 'Saved, type to replace'
                    : 'Paste, or set NGROK_AUTHTOKEN'
                "
              />
            </label>
            <label>
              Domain
              <input v-model="tunnel.domain" type="text" placeholder="Optional reserved domain" />
            </label>
          </template>
          <label v-else>
            Port
            <input v-model.number="tunnel.localPort" type="number" placeholder="7457" />
          </label>
          <button class="ghost-button" :disabled="busy" @click="saveTunnel">Save</button>
        </div>

        <div class="row">
          <template v-if="!status?.running">
            <label>
              Open for
              <select v-model.number="hours">
                <option :value="1">1 hour</option>
                <option :value="4">4 hours</option>
                <option :value="8">8 hours</option>
                <option :value="24">24 hours</option>
              </select>
            </label>
            <button
              class="primary-button"
              :disabled="busy || status?.daemonDown"
              @click="openTunnel"
            >
              {{ busy ? 'Opening...' : 'Open tunnel' }}
            </button>
          </template>
          <button v-else class="ghost-button" :disabled="busy" @click="closeTunnel">
            Close tunnel
          </button>
        </div>
      </section>

      <section v-if="issued" class="issued">
        <h4>Token for {{ issued.label || 'Unnamed' }}</h4>
        <p class="note">
          Shown once. Send it privately. Anyone holding it can read what you picked.
        </p>
        <div class="copy-block">
          <code>{{ issued.token }}</code>
          <button class="ghost-button" @click="copy(issued.token)">Copy</button>
        </div>
        <p v-if="!nodeURL" class="note">
          Open the tunnel to fill in the URL, or replace NODE_URL by hand.
        </p>
        <p class="note">Claude Code</p>
        <div class="copy-block">
          <code>{{ claudeCommand }}</code>
          <button class="ghost-button" @click="copy(claudeCommand)">Copy</button>
        </div>
        <p class="note">Other MCP clients</p>
        <div class="copy-block">
          <pre>{{ mcpJSON }}</pre>
          <button class="ghost-button" @click="copy(mcpJSON)">Copy</button>
        </div>
        <button class="ghost-button" @click="issued = null">Done</button>
      </section>

      <section>
        <h4>People</h4>
        <p v-if="state.grants.length === 0" class="note">Nobody has access yet.</p>
        <div v-for="g in state.grants" :key="g.id" class="grant">
          <div class="grant-main">
            <strong>{{ g.label }}</strong>
            <span class="muted">{{ shareSummary(g) }}</span>
            <span class="muted">
              {{ grantState(g) }}. Expires {{ when(g.expiresAt) }}. Last used
              {{ when(g.lastUsed) }}.
            </span>
          </div>
          <button v-if="!g.revoked" class="ghost-button" :disabled="busy" @click="revoke(g.id)">
            Revoke
          </button>
          <button v-else class="ghost-button" :disabled="busy" @click="remove(g.id)">Remove</button>
        </div>
      </section>

      <section>
        <h4>Give access</h4>
        <div class="row">
          <label>
            Name
            <input v-model="label" type="text" placeholder="Who this token is for" />
          </label>
          <label>
            Days
            <input v-model.number="days" type="number" min="1" />
          </label>
        </div>
        <div v-for="p in state.projects" :key="p.projectId" class="project">
          <button
            class="project-head"
            @click="expanded = expanded === p.projectId ? null : p.projectId"
          >
            {{ p.name }}
            <span class="muted">
              as {{ p.alias }},
              {{
                (picked[p.projectId]?.books.length ?? 0) + (picked[p.projectId]?.tasks.length ?? 0)
              }}
              picked
            </span>
          </button>
          <div v-if="expanded === p.projectId && picked[p.projectId]" class="picks">
            <div class="pick-col">
              <span class="muted">Books</span>
              <p v-if="p.books.length === 0" class="muted">None.</p>
              <label v-for="b in p.books" :key="b" class="check">
                <input v-model="picked[p.projectId].books" type="checkbox" :value="b" />
                {{ b }}
              </label>
            </div>
            <div class="pick-col">
              <span class="muted">Tasks</span>
              <p v-if="p.tasks.length === 0" class="muted">None.</p>
              <label v-for="t in p.tasks" :key="t.slug + t.archived" class="check">
                <input v-model="picked[p.projectId].tasks" type="checkbox" :value="t.slug" />
                {{ t.slug }}<span v-if="t.archived" class="muted"> (archived)</span>
              </label>
            </div>
          </div>
        </div>
        <div class="row">
          <button class="primary-button" :disabled="busy || pickedCount === 0" @click="issue">
            Issue token
          </button>
        </div>
      </section>
    </div>
    <p v-else-if="error" class="error">{{ error }}</p>
  </AppModal>
</template>

<style scoped>
.node {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  font-size: var(--text-sm);
  color: var(--color-fg-primary);
  max-height: 70vh;
  overflow-y: auto;
}

section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border-subtle);
}

h4 {
  margin: 0;
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
}

p {
  margin: 0;
}

.note,
.muted {
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.error {
  color: var(--color-error);
  font-size: var(--text-xs);
}

.status.on {
  color: var(--color-fg-accent);
}

.row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--space-2);
}

label {
  display: flex;
  flex-direction: column;
  gap: 2px;
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

input[type='text'],
input[type='password'],
input[type='number'],
select {
  padding: var(--space-1) var(--space-2);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
}

input:focus,
select:focus {
  outline: none;
  border-color: var(--color-amber);
}

code,
pre {
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  word-break: break-all;
  white-space: pre-wrap;
  margin: 0;
}

.copy-block {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-2);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
}

.copy-block code,
.copy-block pre {
  flex: 1;
  min-width: 0;
}

.issued {
  border: 1px solid var(--color-amber);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}

.grant {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.grant-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.project-head {
  width: 100%;
  display: flex;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2);
  background: var(--color-surface-2);
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
}

.picks {
  display: flex;
  gap: var(--space-4);
  padding: var(--space-2);
}

.pick-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.check {
  flex-direction: row;
  align-items: center;
  gap: var(--space-1);
  color: var(--color-fg-primary);
  word-break: break-all;
}

.ghost-button,
.primary-button {
  padding: var(--space-1) var(--space-4);
  border-radius: var(--radius-pill);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  cursor: pointer;
  flex-shrink: 0;
}

.ghost-button {
  background: transparent;
  border: 1px solid var(--color-border-default);
  color: var(--color-fg-secondary);
}

.ghost-button:hover {
  color: var(--color-fg-primary);
  background: var(--color-surface-2);
}

.primary-button {
  background: var(--color-amber);
  border: none;
  color: var(--color-black);
}

.primary-button:hover {
  background: var(--color-amber-dim);
}

button:disabled {
  opacity: 0.5;
  cursor: default;
}
</style>

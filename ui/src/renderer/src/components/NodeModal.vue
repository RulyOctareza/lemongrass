<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import AppModal from './AppModal.vue'
import AppButton from './AppButton.vue'
import AppCheckbox from './AppCheckbox.vue'
import AppSegmented from './AppSegmented.vue'
import AppSwitcher from './AppSwitcher.vue'
import type {
  NodeGrantView,
  NodeIssued,
  NodeProjectContent,
  NodeShare,
  NodeState
} from '../../../preload/types'

const emit = defineEmits<{
  close: []
}>()

const DAY_MS = 86_400_000

type Section = 'tunnel' | 'access'
type Kind = 'books' | 'tasks' | 'laws' | 'handovers'

const sections: { id: Section; label: string }[] = [
  { id: 'tunnel', label: 'Tunnel' },
  { id: 'access', label: 'Access' }
]

const kinds: { id: Kind; label: string; one: string }[] = [
  { id: 'books', label: 'Books', one: 'Book' },
  { id: 'tasks', label: 'Scratchpad tasks', one: 'Task' },
  { id: 'laws', label: 'Laws', one: 'Law' },
  { id: 'handovers', label: 'Handovers', one: 'Handover' }
]

const providerOptions = [
  { value: 'ngrok' as const, label: 'ngrok' },
  { value: 'local' as const, label: 'This machine only' }
]

const hourOptions = [1, 4, 8, 24].map((h) => ({
  value: h,
  label: `${h} ${h === 1 ? 'hour' : 'hours'}`
}))

const state = ref<NodeState | null>(null)
const error = ref('')
const busy = ref(false)
const section = ref<Section>('access')

const tunnel = reactive({
  provider: 'ngrok' as 'ngrok' | 'local',
  authtoken: '',
  domain: '',
  localPort: 0
})
const hours = ref(8)

const selectedId = ref<string | null>(null)
// A new token is a blank draft with no id until Save creates it.
const creating = ref(false)
const previousId = ref<string | null>(null)
const savedNotice = ref('')
let savedTimer: ReturnType<typeof setTimeout> | undefined
const issued = ref<(NodeIssued & { label: string }) | null>(null)
const confirming = ref<string | null>(null)
const copied = ref('')
let copiedTimer: ReturnType<typeof setTimeout> | undefined

// The selected token's edits, applied only by Save.
interface Draft {
  label: string
  expiresAt: string
  shares: NodeShare[]
}
const draft = ref<Draft | null>(null)
const baseline = ref('')
const pendingSwitch = ref<string | null>(null)

const addingResource = ref(false)
const resourceProjectId = ref<string | null>(null)
const resourceFilter = ref('')
const openKinds = ref<Kind[]>([])

// Only a real edit counts, so an untouched new token can be left without a prompt.
const dirty = computed(() => draft.value !== null && JSON.stringify(draft.value) !== baseline.value)

const renaming = ref(false)
const renameInput = ref<HTMLInputElement | null>(null)

const selected = computed(
  (): NodeGrantView | null => state.value?.grants.find((g) => g.id === selectedId.value) ?? null
)

function apply(next: NodeState): void {
  state.value = next
  tunnel.provider = next.tunnel.provider
  tunnel.domain = next.tunnel.domain
  tunnel.localPort = next.tunnel.localPort
  resourceProjectId.value ??= next.projects[0]?.projectId ?? null
  if (creating.value) return
  if (!next.grants.some((g) => g.id === selectedId.value)) {
    selectedId.value = next.grants[0]?.id ?? null
    loadDraft()
  } else if (!dirty.value) {
    loadDraft()
  }
}

function loadDraft(): void {
  const g = selected.value
  draft.value =
    g && !g.revoked
      ? {
          label: g.label,
          expiresAt: g.expiresAt,
          shares: JSON.parse(JSON.stringify(g.shares)) as NodeShare[]
        }
      : null
  baseline.value = JSON.stringify(draft.value)
  pendingSwitch.value = null
}

function messageOf(err: unknown): string {
  return err instanceof Error
    ? err.message.replace(/^Error invoking remote method '[^']+': (Error: )?/, '')
    : String(err)
}

async function run(fn: () => Promise<NodeState | void>): Promise<void> {
  busy.value = true
  error.value = ''
  try {
    const next = await fn()
    if (next) apply(next)
  } catch (err) {
    error.value = messageOf(err)
  } finally {
    busy.value = false
  }
}

onMounted(() => run(() => window.api.node.state()))
onUnmounted(() => {
  clearTimeout(copiedTimer)
  clearTimeout(savedTimer)
})

function selectSection(id: Section): void {
  section.value = id
  error.value = ''
}

// Saves the typed tunnel settings first, so Open never ignores what was typed.
function openTunnel(): Promise<void> {
  return run(async () => {
    await window.api.node.saveTunnel(
      tunnel.provider,
      tunnel.authtoken ? tunnel.authtoken : null,
      tunnel.domain,
      tunnel.localPort
    )
    tunnel.authtoken = ''
    return window.api.node.openTunnel(hours.value)
  })
}

const closeTunnel = (): Promise<void> => run(() => window.api.node.closeTunnel())

const status = computed(() => state.value?.status)
const nodeURL = computed(() => status.value?.url ?? '')

function grantState(g: NodeGrantView): 'Active' | 'Expired' | 'Disconnected' {
  if (g.revoked) return 'Disconnected'
  if (new Date(g.expiresAt).getTime() <= Date.now()) return 'Expired'
  return 'Active'
}

const tokenItems = computed(() =>
  (state.value?.grants ?? []).map((g) => {
    const s = grantState(g)
    return { id: g.id, label: s === 'Active' ? g.label : `${g.label} (${s.toLowerCase()})` }
  })
)

// An unsaved rename shows in the closed dropdown; the list keeps the saved names.
const pendingName = computed(() =>
  !creating.value && selected.value && draft.value && draft.value.label !== selected.value.label
    ? draft.value.label
    : undefined
)

async function startRename(): Promise<void> {
  renaming.value = true
  await nextTick()
  renameInput.value?.focus()
  renameInput.value?.select()
}

function endRename(): void {
  if (draft.value && !draft.value.label.trim()) draft.value.label = selected.value?.label ?? ''
  renaming.value = false
}

// Switching away from unsaved changes waits for Save or Discard.
function selectToken(id: string): void {
  if (id === selectedId.value && !creating.value) return
  if (dirty.value) {
    pendingSwitch.value = id
    return
  }
  switchTo(id)
}

function switchTo(id: string): void {
  creating.value = false
  renaming.value = false
  selectedId.value = id
  confirming.value = null
  addingResource.value = false
  if (issued.value?.id !== id) issued.value = null
  loadDraft()
}

function startAddToken(): void {
  if (dirty.value) {
    pendingSwitch.value = null
    error.value = 'Save or discard these changes before adding a token.'
    return
  }
  error.value = ''
  if (!creating.value) previousId.value = selectedId.value
  creating.value = true
  renaming.value = false
  confirming.value = null
  addingResource.value = false
  issued.value = null
  draft.value = {
    label: '',
    expiresAt: new Date(Date.now() + 30 * DAY_MS).toISOString(),
    shares: []
  }
  baseline.value = JSON.stringify(draft.value)
}

function noteSaved(): void {
  savedNotice.value = 'Saved'
  clearTimeout(savedTimer)
  savedTimer = setTimeout(() => (savedNotice.value = ''), 2500)
}

// The one save of the Access section: creates the token first when it is new, then writes its
// name, expiry and resources.
async function save(): Promise<void> {
  const d = draft.value
  if (!d || (!creating.value && !selected.value)) return
  if (!d.label.trim()) {
    error.value = 'The token needs a name.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    // IPC cannot clone Vue's reactive proxies, so the draft crosses as fresh plain objects.
    const shares = d.shares
      .filter((s) => shareSize(s) > 0)
      .map((s) => ({
        alias: s.alias,
        books: [...s.books],
        tasks: [...s.tasks],
        laws: [...s.laws],
        handovers: [...s.handovers]
      }))
    let id = selectedId.value!
    if (creating.value) {
      const result = await window.api.node.issue({ label: d.label.trim() })
      issued.value = { ...result, label: d.label.trim() }
      id = result.id
    }
    const next = await window.api.node.update({
      id,
      label: d.label,
      expiresAt: d.expiresAt,
      shares
    })
    const target = pendingSwitch.value
    creating.value = false
    renaming.value = false
    selectedId.value = id
    baseline.value = JSON.stringify(draft.value)
    apply(next)
    loadDraft()
    noteSaved()
    if (target && target !== id) switchTo(target)
  } catch (err) {
    error.value = messageOf(err)
  } finally {
    busy.value = false
  }
}

function discard(): void {
  const target = pendingSwitch.value
  error.value = ''
  renaming.value = false
  if (creating.value) {
    creating.value = false
    selectedId.value = previousId.value
  }
  loadDraft()
  if (target) switchTo(target)
}

const footerStatus = computed((): { text: string; tone: '' | 'muted' | 'saved' | 'error' } => {
  if (error.value) return { text: error.value, tone: 'error' }
  if (pendingSwitch.value) {
    return { text: 'Save or discard these changes before switching tokens.', tone: '' }
  }
  if (dirty.value) return { text: 'Unsaved changes', tone: '' }
  if (creating.value) return { text: 'New token', tone: 'muted' }
  if (savedNotice.value) return { text: savedNotice.value, tone: 'saved' }
  return { text: 'No changes', tone: 'muted' }
})

// Disconnect and Remove act on the second click, the same as removing an account in Settings.
function confirmThen(key: string, fn: () => Promise<NodeState>): void {
  if (confirming.value !== key) {
    confirming.value = key
    return
  }
  confirming.value = null
  void run(async () => {
    const next = await fn()
    baseline.value = JSON.stringify(draft.value)
    return next
  })
}

const disconnect = (id: string): Promise<NodeState> => window.api.node.revoke(id)
const removeToken = (id: string): Promise<NodeState> => window.api.node.remove(id)

function daysLeft(expiresAt: string): number {
  return Math.max(1, Math.ceil((new Date(expiresAt).getTime() - Date.now()) / DAY_MS))
}

function setDays(value: number): void {
  if (!draft.value) return
  if (!Number.isFinite(value) || value < 1) {
    error.value = 'The expiry needs at least one day.'
    return
  }
  draft.value.expiresAt = new Date(Date.now() + value * DAY_MS).toISOString()
}

function when(iso?: string): string {
  if (!iso) return 'never'
  return new Date(iso).toLocaleString()
}

function shareSize(s: NodeShare): number {
  return s.books.length + s.tasks.length + s.laws.length + s.handovers.length
}

const projectByAlias = computed(
  () => new Map((state.value?.projects ?? []).map((p) => [p.alias, p]))
)

const draftShares = computed(() => (draft.value?.shares ?? []).filter((s) => shareSize(s) > 0))

function shareFor(alias: string): NodeShare {
  const d = draft.value!
  let share = d.shares.find((s) => s.alias === alias)
  if (!share) {
    share = { alias, books: [], tasks: [], laws: [], handovers: [] }
    d.shares.push(share)
  }
  return share
}

function has(alias: string, kind: Kind, name: string): boolean {
  return draft.value?.shares.find((s) => s.alias === alias)?.[kind].includes(name) ?? false
}

function toggle(alias: string, kind: Kind, name: string): void {
  const share = shareFor(alias)
  share[kind] = share[kind].includes(name)
    ? share[kind].filter((n) => n !== name)
    : [...share[kind], name]
}

function removeResource(alias: string, kind: Kind, name: string): void {
  const share = shareFor(alias)
  share[kind] = share[kind].filter((n) => n !== name)
}

const resourceProject = computed((): NodeProjectContent | undefined =>
  state.value?.projects.find((p) => p.projectId === resourceProjectId.value)
)

const projectItems = computed(() =>
  (state.value?.projects ?? []).map((p) => ({ id: p.projectId, label: p.alias }))
)

// Every domain lists the names a grant stores, with archived ones marked for the owner only.
function options(p: NodeProjectContent, kind: Kind): { name: string; label: string }[] {
  const query = resourceFilter.value.trim().toLowerCase()
  const all =
    kind === 'books'
      ? p.books.map((name) => ({ name, label: name }))
      : kind === 'tasks'
        ? p.tasks.map((t) => ({
            name: t.slug,
            label: t.archived ? `${t.slug} (archived)` : t.slug
          }))
        : p[kind].map((d) => ({
            name: d.name,
            label: d.archived ? `${d.name} (archived)` : d.name
          }))
  return all.filter((o) => o.label.toLowerCase().includes(query))
}

function total(p: NodeProjectContent, kind: Kind): number {
  return p[kind].length
}

function pickedIn(alias: string, kind: Kind): number {
  return draft.value?.shares.find((s) => s.alias === alias)?.[kind].length ?? 0
}

function toggleKind(kind: Kind): void {
  openKinds.value = openKinds.value.includes(kind)
    ? openKinds.value.filter((k) => k !== kind)
    : [...openKinds.value, kind]
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

const claudeCommand = computed(() =>
  issued.value
    ? `claude mcp add --transport http ${serverName(issued.value.label)} ${nodeURL.value || 'NODE_URL'}/mcp --header "Authorization: Bearer ${issued.value.token}"`
    : ''
)

function copy(key: string, text: string): void {
  void navigator.clipboard.writeText(text)
  copied.value = key
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copied.value = ''), 1500)
}
</script>

<template>
  <AppModal scope="app" size="lg" flush label="Node" @close="emit('close')">
    <div class="settings-layout">
      <aside class="settings-nav">
        <h3 class="settings-nav-title">Node</h3>
        <button
          v-for="item in sections"
          :key="item.id"
          type="button"
          class="settings-nav-item"
          :class="{ selected: section === item.id }"
          @click="selectSection(item.id)"
        >
          {{ item.label }}
        </button>
      </aside>

      <section v-if="!state" class="settings-section">
        <p v-if="error" class="settings-error">{{ error }}</p>
        <p v-else class="settings-text">Loading...</p>
      </section>

      <section v-else-if="section === 'tunnel'" class="settings-section">
        <h4 class="settings-title">Tunnel</h4>
        <p class="settings-text">
          Makes this node reachable from outside your machine. It closes by itself when its time is
          up.
        </p>
        <p v-if="error" class="settings-error">{{ error }}</p>

        <ul class="settings-rows">
          <li class="settings-row">
            <span class="node-dot" :class="{ on: status?.running }" aria-hidden="true"></span>
            <span class="settings-row-title">{{ status?.running ? 'Open' : 'Closed' }}</span>
            <span v-if="status?.daemonDown" class="settings-row-meta">
              The node daemon is not running. Restart the app to start it.
            </span>
            <span v-else-if="status?.running" class="settings-row-meta" :title="nodeURL">
              {{ nodeURL }}, until {{ when(status.until) }}
            </span>
            <span v-else class="settings-row-meta">Nobody outside this machine can reach it.</span>
            <template v-if="status?.running">
              <button type="button" class="settings-action" @click="copy('url', nodeURL)">
                {{ copied === 'url' ? 'Copied' : 'Copy URL' }}
              </button>
              <button type="button" class="settings-action" :disabled="busy" @click="closeTunnel">
                Close
              </button>
            </template>
          </li>
        </ul>
        <p v-if="status?.error" class="settings-error">{{ status.error }}</p>

        <div class="settings-field">
          <span id="node-provider">Provider</span>
          <AppSegmented
            v-model="tunnel.provider"
            :options="providerOptions"
            aria-labelledby="node-provider"
          />
        </div>
        <div v-if="tunnel.provider === 'ngrok'" class="settings-fields">
          <label class="settings-field">
            Authtoken
            <input
              v-model="tunnel.authtoken"
              class="settings-input"
              type="password"
              autocomplete="off"
              :placeholder="
                state.tunnel.authtokenSet
                  ? 'Saved, type to replace'
                  : 'Paste, or set NGROK_AUTHTOKEN'
              "
            />
          </label>
          <label class="settings-field">
            Domain
            <input
              v-model="tunnel.domain"
              class="settings-input"
              type="text"
              spellcheck="false"
              placeholder="Optional"
            />
          </label>
        </div>
        <div v-else class="settings-fields">
          <label class="settings-field settings-field--narrow">
            Port
            <input
              v-model.number="tunnel.localPort"
              class="settings-input"
              type="number"
              placeholder="7457"
            />
          </label>
        </div>

        <template v-if="!status?.running">
          <div class="settings-field">
            <span id="node-hours">Open for</span>
            <AppSegmented v-model="hours" :options="hourOptions" aria-labelledby="node-hours" />
          </div>
          <div class="settings-add">
            <AppButton variant="primary" :disabled="busy || status?.daemonDown" @click="openTunnel">
              {{ busy ? 'Opening...' : 'Open tunnel' }}
            </AppButton>
          </div>
          <p class="settings-text">Opening saves these settings first.</p>
        </template>
      </section>

      <section v-else class="settings-section settings-section--fixed">
        <h4 class="settings-title">Access</h4>
        <p class="settings-text">
          Each person gets one token and sets it up once. Change what it can read at any time. A
          change applies on their next request, with no new setup.
        </p>

        <div class="settings-fields">
          <div class="settings-field">
            <span>Token</span>
            <div class="node-token">
              <input
                v-if="renaming && draft"
                ref="renameInput"
                v-model="draft.label"
                class="settings-input"
                type="text"
                maxlength="64"
                spellcheck="false"
                aria-label="Token name"
                @keydown.enter="endRename"
                @blur="endRename"
              />
              <AppSwitcher
                v-else
                :items="tokenItems"
                :active-id="creating ? null : selectedId"
                :label="pendingName"
                :placeholder="creating ? 'New token' : 'No token yet'"
                add-label="Add token"
                empty-text="No tokens yet."
                framed
                @select="selectToken"
                @add="startAddToken"
              />
              <button
                v-if="selected && !selected.revoked && !creating && !renaming"
                type="button"
                class="settings-action"
                @click="startRename"
              >
                Rename
              </button>
            </div>
          </div>
        </div>

        <div class="settings-scroll">
          <div v-if="issued && issued.id === selectedId" class="node-issued">
            <div class="node-issued-head">
              <span class="settings-row-title">Setup for {{ issued.label }}</span>
              <button type="button" class="settings-action" @click="issued = null">Done</button>
            </div>
            <p class="settings-text">
              Shown once. Send it privately. They add it once, and what it can read is changed here
              without a new setup.
            </p>
            <span class="node-copy-label">Token</span>
            <div class="node-copy">
              <code>{{ issued.token }}</code>
              <button type="button" class="settings-action" @click="copy('token', issued.token)">
                {{ copied === 'token' ? 'Copied' : 'Copy' }}
              </button>
            </div>
            <span class="node-copy-label">Claude Code</span>
            <div class="node-copy">
              <code>{{ claudeCommand }}</code>
              <button type="button" class="settings-action" @click="copy('claude', claudeCommand)">
                {{ copied === 'claude' ? 'Copied' : 'Copy' }}
              </button>
            </div>
            <p v-if="!nodeURL" class="settings-text">
              Open the tunnel to fill in the URL, or replace NODE_URL by hand.
            </p>
          </div>

          <template v-if="!creating && selected && selected.revoked">
            <ul class="settings-rows" @mouseleave="confirming = null">
              <li class="settings-row">
                <span class="settings-row-title">{{ selected.label }}</span>
                <span class="settings-row-meta">This token no longer works.</span>
                <span class="settings-badge">Disconnected</span>
                <button
                  type="button"
                  class="settings-action"
                  :class="{ confirming: confirming === `remove:${selected.id}` }"
                  :disabled="busy"
                  @click="confirmThen(`remove:${selected.id}`, () => removeToken(selected!.id))"
                >
                  {{ confirming === `remove:${selected.id}` ? 'Click again' : 'Remove' }}
                </button>
              </li>
            </ul>
          </template>

          <template v-else-if="draft && (creating || selected)">
            <div v-if="creating" class="settings-fields">
              <label class="settings-field">
                Name
                <input
                  v-model="draft.label"
                  class="settings-input"
                  type="text"
                  maxlength="64"
                  spellcheck="false"
                  placeholder="Who this token is for"
                />
              </label>
            </div>
            <dl class="settings-facts">
              <template v-if="selected && !creating">
                <dt>Status</dt>
                <dd>
                  <span class="settings-badge" :class="{ ok: grantState(selected) === 'Active' }">
                    {{ grantState(selected) }}
                  </span>
                </dd>
              </template>
              <dt>Expires</dt>
              <dd class="node-expires">
                in
                <input
                  class="settings-input node-days"
                  type="number"
                  min="1"
                  aria-label="Days until it expires"
                  :value="daysLeft(draft.expiresAt)"
                  @change="setDays(($event.target as HTMLInputElement).valueAsNumber)"
                />
                days
              </dd>
              <template v-if="selected && !creating">
                <dt>Last used</dt>
                <dd>{{ selected.lastUsed ? when(selected.lastUsed) : 'Not used yet' }}</dd>
              </template>
            </dl>

            <h5 class="settings-subtitle">Can read</h5>
            <p v-if="draftShares.length === 0" class="settings-text">
              Nothing yet. The token works and its contents are empty.
            </p>
            <template v-for="share in draftShares" :key="share.alias">
              <span class="settings-subtitle">
                {{ share.alias }}
                <template v-if="!projectByAlias.has(share.alias)">, no longer registered</template>
              </span>
              <ul class="settings-rows">
                <template v-for="kind in kinds" :key="kind.id">
                  <li v-for="name in share[kind.id]" :key="kind.id + name" class="settings-row">
                    <span class="settings-badge">{{ kind.one }}</span>
                    <span class="settings-row-title node-name">{{ name }}</span>
                    <button
                      type="button"
                      class="settings-action"
                      @click="removeResource(share.alias, kind.id, name)"
                    >
                      Remove
                    </button>
                  </li>
                </template>
              </ul>
            </template>

            <div v-if="!addingResource" class="settings-add">
              <AppButton :disabled="!state.projects.length" @click="addingResource = true">
                + Add resource
              </AppButton>
            </div>
            <div v-else class="node-add">
              <div class="settings-fields">
                <div class="settings-field">
                  <span>Project</span>
                  <AppSwitcher
                    :items="projectItems"
                    :active-id="resourceProjectId"
                    placeholder="Pick a project"
                    empty-text="No projects registered."
                    framed
                    @select="(id) => (resourceProjectId = id)"
                  />
                </div>
                <label class="settings-field">
                  Filter
                  <input
                    v-model="resourceFilter"
                    class="settings-input"
                    type="text"
                    spellcheck="false"
                    placeholder="Any name"
                  />
                </label>
              </div>
              <template v-if="resourceProject">
                <div v-for="kind in kinds" :key="kind.id" class="settings-group">
                  <button
                    type="button"
                    class="settings-group-head"
                    :aria-expanded="openKinds.includes(kind.id)"
                    @click="toggleKind(kind.id)"
                  >
                    <span
                      class="settings-group-chevron"
                      :class="{ expanded: openKinds.includes(kind.id) }"
                      aria-hidden="true"
                      >▸</span
                    >
                    {{ kind.label }}
                    <span class="settings-group-count">
                      {{ pickedIn(resourceProject.alias, kind.id) }} of
                      {{ total(resourceProject, kind.id) }}
                    </span>
                  </button>
                  <div v-if="openKinds.includes(kind.id)" class="settings-group-body">
                    <p v-if="total(resourceProject, kind.id) === 0" class="settings-text">None.</p>
                    <p
                      v-else-if="options(resourceProject, kind.id).length === 0"
                      class="settings-text"
                    >
                      Nothing matches.
                    </p>
                    <AppCheckbox
                      v-for="o in options(resourceProject, kind.id)"
                      :key="o.name"
                      :checked="has(resourceProject.alias, kind.id, o.name)"
                      @click="toggle(resourceProject.alias, kind.id, o.name)"
                    >
                      {{ o.label }}
                    </AppCheckbox>
                  </div>
                </div>
              </template>
              <div class="settings-add">
                <AppButton @click="addingResource = false">Done</AppButton>
              </div>
            </div>

            <ul v-if="selected && !creating" class="settings-rows node-danger">
              <li class="settings-row" @mouseleave="confirming = null">
                <span class="settings-row-title">Disconnect</span>
                <span class="settings-row-meta">
                  Ends this token. They would need a new setup.
                </span>
                <button
                  type="button"
                  class="settings-action"
                  :class="{ confirming: confirming === `revoke:${selected.id}` }"
                  :disabled="busy"
                  @click="confirmThen(`revoke:${selected.id}`, () => disconnect(selected!.id))"
                >
                  {{ confirming === `revoke:${selected.id}` ? 'Click again' : 'Disconnect' }}
                </button>
              </li>
            </ul>
          </template>
        </div>

        <div class="settings-footer">
          <span class="settings-footer-status" :class="footerStatus.tone">
            {{ footerStatus.text }}
          </span>
          <AppButton v-if="dirty" :disabled="busy" @click="discard">Discard</AppButton>
          <AppButton
            variant="primary"
            :disabled="!dirty || busy || (creating && !draft?.label.trim())"
            @click="save"
          >
            {{ busy ? 'Saving...' : creating ? 'Create token' : 'Save token' }}
          </AppButton>
        </div>
      </section>
    </div>
  </AppModal>
</template>

<style scoped>
.node-dot {
  width: 8px;
  height: 8px;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--color-fg-muted);
}

.node-dot.on {
  background: var(--color-amber);
}

.node-token {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.node-token > :first-child {
  flex: 1;
  min-width: 0;
}

.node-expires {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.node-days {
  width: 5rem;
  padding-top: var(--space-1);
  padding-bottom: var(--space-1);
}

.node-danger {
  margin-top: var(--space-4);
}

.node-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-add {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  --input-bg: var(--color-surface-1);
}

.node-issued {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--color-amber);
  border-radius: var(--radius-md);
}

.node-issued-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.node-copy-label {
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.node-copy {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-0);
  border-radius: var(--radius-md);
}

.node-copy code {
  flex: 1;
  min-width: 0;
  margin: 0;
  color: var(--color-fg-primary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  white-space: pre-wrap;
  word-break: break-all;
}
</style>

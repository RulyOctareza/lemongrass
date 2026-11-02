<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import '../../resterClient.css'
import AppButton from '../AppButton.vue'
import AppSwitcher from '../AppSwitcher.vue'
import VaultUnlockGate from '../VaultUnlockGate.vue'
import RequestList from './RequestList.vue'
import RequestEditor from './RequestEditor.vue'
import ResponsePanel from './ResponsePanel.vue'
import { describeError, noteVaultActivity, vaultSession } from '../../vaultSession'
import { plainCopy } from '../connector/rester/resterDraft'
import { looksLikeCurl, matchBaseURL, parseCurl, type BaseURLTarget } from '../../curlRequest'
import { apiUsers, buildCall } from '../../resterClientCall'
import {
  clearHistory,
  deleteRun,
  loadHistory,
  loadResterClient,
  recordRun,
  removeRequest as removeStoredRequest,
  resterClientStore
} from '../../resterClientStore'
import type {
  ResterClientDownload,
  ResterClientRequest,
  ResterClientResult,
  ResterClientRun,
  VaultRester
} from '../../../../preload/types'
import AppInput from '../AppInput.vue'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
// Where the last selected request is remembered, so reopening the app shows it again.
const SELECTED_KEY = 'resterClient.selectedId'

interface Outcome {
  result: ResterClientResult | null
  download: ResterClientDownload | null
  error: string
  sending: boolean
  notice: string
  // The run the outcome shows and when it came back, null before any run.
  runId: string | null
  at: string | null
  bodyCut: boolean
  // Set once the newest stored run has been looked up for this request.
  restored: boolean
}

const requests = computed(() => resterClientStore.requests)
const selectedId = ref<string | null>(null)
const outcomes = reactive<Record<string, Outcome>>({})

const runs = computed(() =>
  selectedId.value ? (resterClientStore.histories[selectedId.value] ?? []) : []
)

const resters = ref<Record<string, VaultRester>>({})
const restersError = ref('')
const restersLoading = ref(false)

const selected = computed(() => requests.value.find((r) => r.id === selectedId.value) ?? null)

function outcomeOf(id: string): Outcome {
  if (!outcomes[id]) {
    outcomes[id] = {
      result: null,
      download: null,
      error: '',
      sending: false,
      notice: '',
      runId: null,
      at: null,
      bodyCut: false,
      restored: false
    }
  }
  return outcomes[id]
}

const outcome = computed(() => (selected.value ? outcomeOf(selected.value.id) : null))

// Every rester environment with an api side, which is what a request can target.
const targets = computed<BaseURLTarget[]>(() =>
  Object.entries(resters.value).flatMap(([name, rester]) =>
    rester.API
      ? rester.Environments.filter((env) => env.BaseURL).map((env) => ({
          rester: name,
          environment: env.Name,
          baseURL: env.BaseURL
        }))
      : []
  )
)

function targetId(rester: string, environment: string): string {
  return JSON.stringify([rester, environment])
}

const targetItems = computed(() =>
  targets.value.map((t) => ({
    id: targetId(t.rester, t.environment),
    label: `${t.rester} · ${t.environment}`,
    title: t.baseURL
  }))
)

const selectedTarget = computed(() =>
  selected.value?.rester ? targetId(selected.value.rester, selected.value.environment) : null
)

const baseURL = computed(
  () =>
    targets.value.find(
      (t) => t.rester === selected.value?.rester && t.environment === selected.value?.environment
    )?.baseURL ?? ''
)

const userItems = computed(() => {
  const rester = selected.value ? resters.value[selected.value.rester] : undefined
  if (!rester || !selected.value) return []
  return apiUsers(rester, selected.value.environment).map((user) => ({
    id: user.Name,
    label: user.Tags?.length ? `${user.Name} (${user.Tags.join(', ')})` : user.Name,
    search: user.Name
  }))
})

const resolvedURL = computed(() => {
  const url = selected.value?.url.trim() ?? ''
  if (!url || url.includes('://') || !baseURL.value) return ''
  return baseURL.value.replace(/\/+$/, '') + (url.startsWith('/') ? url : '/' + url)
})

const canSend = computed(
  () =>
    !!selected.value &&
    !!selected.value.rester &&
    !!selected.value.user &&
    !!selected.value.url.trim() &&
    !outcome.value?.sending
)

async function loadResters(): Promise<void> {
  restersLoading.value = true
  restersError.value = ''
  try {
    const names = await window.api.vault.listResters()
    const loadedResters: Record<string, VaultRester> = {}
    for (const name of names) {
      loadedResters[name] = await window.api.vault.getRester(vaultSession.passphrase, name)
    }
    resters.value = loadedResters
  } catch (err) {
    restersError.value = describeError(err)
  } finally {
    restersLoading.value = false
  }
}

watch(
  () => vaultSession.unlocked,
  (unlocked) => {
    if (unlocked) loadResters()
  },
  { immediate: true }
)

onMounted(async () => {
  await loadResterClient()
  const remembered = localStorage.getItem(SELECTED_KEY)
  selectedId.value ??= requests.value.some((r) => r.id === remembered)
    ? remembered
    : (requests.value[0]?.id ?? null)
})

watch(selectedId, async (id) => {
  if (!id) return
  localStorage.setItem(SELECTED_KEY, id)
  await loadHistory(id)
  const state = outcomeOf(id)
  if (state.restored) return
  state.restored = true
  const newest = resterClientStore.histories[id]?.[0]
  // A send that started before the history arrived already shows its own run.
  if (newest && !state.runId && !state.sending) showRun(state, newest)
})

function addRequest(): void {
  const current = selected.value
  const request: ResterClientRequest = {
    id: crypto.randomUUID(),
    name: '',
    rester: current?.rester ?? targets.value[0]?.rester ?? '',
    environment: current?.environment ?? targets.value[0]?.environment ?? '',
    user: current?.user ?? '',
    method: 'GET',
    url: '',
    headers: [],
    bodyKind: 'none',
    json: '',
    form: []
  }
  resterClientStore.requests.push(request)
  selectedId.value = request.id
  if (!request.user) pickDefaultUser(request)
}

function removeRequest(id: string): void {
  const index = requests.value.findIndex((r) => r.id === id)
  if (index < 0) return
  removeStoredRequest(id)
  delete outcomes[id]
  if (selectedId.value === id) {
    selectedId.value = requests.value[Math.min(index, requests.value.length - 1)]?.id ?? null
  }
}

function renameRequest(id: string, name: string): void {
  const request = requests.value.find((r) => r.id === id)
  if (request) request.name = name
}

// Keeps the request's user when the new environment has it, else takes the first usable one.
function pickDefaultUser(request: ResterClientRequest): void {
  const rester = resters.value[request.rester]
  const users = rester ? apiUsers(rester, request.environment) : []
  if (!users.some((u) => u.Name === request.user)) request.user = users[0]?.Name ?? ''
}

function pickTarget(id: string): void {
  if (!selected.value) return
  const [rester, environment] = JSON.parse(id) as [string, string]
  selected.value.rester = rester
  selected.value.environment = environment
  pickDefaultUser(selected.value)
}

function tokenHeaderOf(request: ResterClientRequest): string {
  const placement = resters.value[request.rester]?.API?.TokenPlacement
  return placement?.Kind === 'header' ? placement.Name : ''
}

// A pasted curl command fills the selected request in place rather than adding a new one.
function onUrlPaste(event: ClipboardEvent): void {
  const text = event.clipboardData?.getData('text/plain') ?? ''
  if (!selected.value || !looksLikeCurl(text)) return
  event.preventDefault()
  const request = selected.value
  const match = matchBaseURL(parseCurl(text).url, targets.value)
  if (match) {
    request.rester = match.target.rester
    request.environment = match.target.environment
    pickDefaultUser(request)
  }
  const parsed = parseCurl(text, tokenHeaderOf(request))
  request.method = parsed.method
  request.url = match ? match.path : parsed.url
  request.headers = parsed.headers
  request.bodyKind = parsed.bodyKind
  request.json = parsed.json
  request.form = parsed.form

  const notes: string[] = []
  if (parsed.dropped.length) {
    notes.push(`Left out ${parsed.dropped.join(', ')}, since the chosen user supplies auth.`)
  }
  if (match) {
    notes.push(`Matched ${match.target.rester} · ${match.target.environment}.`)
  } else if (parsed.url.includes('://')) {
    notes.push('The URL is under no rester environment, so it cannot be sent as it is.')
  }
  notes.push(...parsed.notes)
  outcomeOf(request.id).notice = notes.join(' ')
}

function snapshot(request: ResterClientRequest): ResterClientRun['request'] {
  const copy = plainCopy(request) as Partial<ResterClientRequest>
  delete copy.id
  delete copy.name
  return copy as ResterClientRun['request']
}

async function send(download: boolean): Promise<void> {
  const request = selected.value
  if (!request || !canSend.value) return
  const state = outcomeOf(request.id)
  state.sending = true
  state.error = ''
  state.notice = ''
  state.runId = crypto.randomUUID()
  state.at = null
  state.bodyCut = false
  noteVaultActivity()
  const sent = snapshot(request)
  const runId = state.runId
  const at = new Date().toISOString()
  try {
    const call = plainCopy(buildCall(request))
    if (download) {
      const result = await window.api.resterClient.download(vaultSession.passphrase, call)
      state.download = result.kind === 'inline' ? null : result
      state.result = result.kind === 'inline' ? result.result : null
    } else {
      state.result = await window.api.resterClient.request(vaultSession.passphrase, call)
      state.download = null
    }
  } catch (err) {
    state.error = describeError(err)
    state.result = null
    state.download = null
  } finally {
    state.sending = false
  }
  if (state.download?.kind === 'cancelled') return
  state.at = at
  recordRun(request.id, {
    id: runId,
    at,
    request: sent,
    result: state.result,
    download: state.download,
    error: state.error,
    bodyCut: false
  }).catch((err) => (state.notice = `The run was not kept in history: ${describeError(err)}`))
}

function showRun(state: Outcome, run: ResterClientRun): void {
  state.result = run.result
  state.download = run.download
  state.error = run.error
  state.notice = ''
  state.runId = run.id
  state.at = run.at
  state.bodyCut = run.bodyCut
}

// Loads a run's parameters into its request and shows the response it got.
function pickRun(run: ResterClientRun): void {
  const request = selected.value
  if (!request) return
  Object.assign(request, plainCopy(run.request))
  showRun(outcomeOf(request.id), run)
}

function removeRun(runId: string): void {
  if (selectedId.value) deleteRun(selectedId.value, runId)
}

function clearRuns(): void {
  if (selectedId.value) clearHistory(selectedId.value)
}

async function flush(): Promise<void> {
  const request = selected.value
  if (!request?.rester || !request.user) return
  const state = outcomeOf(request.id)
  try {
    await window.api.resterClient.flush(request.rester, request.environment, request.user)
    state.notice = `Dropped ${request.user}'s token. The next send logs in again.`
  } catch (err) {
    state.notice = describeError(err)
  }
}

function onUrlKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter') send(event.shiftKey)
}
</script>

<template>
  <div
    class="rester-client"
    @click.capture="noteVaultActivity"
    @keydown.capture="noteVaultActivity"
  >
    <div v-if="!vaultSession.unlocked" class="vault-tab">
      <div class="vault-frame vault-frame--gate">
        <VaultUnlockGate />
      </div>
    </div>

    <div v-else class="client-frame">
      <nav class="client-sidebar">
        <div class="sidebar-actions">
          <span class="sidebar-label">Requests</span>
          <AppButton @click="addRequest">+ Request</AppButton>
        </div>
        <RequestList
          :requests="requests"
          :selected-id="selectedId"
          @select="selectedId = $event"
          @remove="removeRequest"
          @rename="renameRequest"
        />
      </nav>

      <section v-if="selected && outcome" class="client-main">
        <div class="target-row">
          <div class="target-picker">
            <AppSwitcher
              framed
              :items="targetItems"
              :active-id="selectedTarget"
              :label="selected.rester ? undefined : 'Pick a rester'"
              placeholder="Pick a rester"
              :empty-text="
                restersLoading ? 'Loading resters...' : 'No rester has an API environment yet.'
              "
              @select="pickTarget"
            />
          </div>
          <div class="user-picker">
            <AppSwitcher
              framed
              searchable
              search-placeholder="Search users"
              :items="userItems"
              :active-id="selected.user || null"
              placeholder="Pick a user"
              empty-text="No user can call this environment."
              @select="selected.user = $event"
            />
          </div>
          <AppButton
            :disabled="!selected.user"
            title="Drop this user's cached token"
            @click="flush"
          >
            Flush token
          </AppButton>
        </div>
        <p v-if="restersError" class="error-text">{{ restersError }}</p>

        <div class="url-row">
          <div class="method-picker">
            <AppSwitcher
              framed
              :items="METHODS.map((m) => ({ id: m, label: m }))"
              :active-id="selected.method"
              placeholder="GET"
              empty-text=""
              @select="selected.method = $event"
            />
          </div>
          <AppInput
            v-model="selected.url"
            mono
            class="url-input"
            type="text"
            spellcheck="false"
            placeholder="/path, a full URL, or paste a curl command"
            @paste="onUrlPaste"
            @keydown="onUrlKeydown"
          />
          <AppButton variant="primary" :disabled="!canSend" @click="send(false)">Send</AppButton>
          <AppButton :disabled="!canSend" @click="send(true)">Send and download</AppButton>
        </div>
        <p v-if="resolvedURL" class="hint resolved">{{ resolvedURL }}</p>
        <p v-if="outcome.notice" class="hint notice">{{ outcome.notice }}</p>

        <div class="panes">
          <RequestEditor :model-value="selected" />
          <ResponsePanel
            :result="outcome.result"
            :download="outcome.download"
            :error="outcome.error"
            :sending="outcome.sending"
            :body-cut="outcome.bodyCut"
            :at="outcome.at"
            :runs="runs"
            :run-id="outcome.runId"
            @pick-run="pickRun"
            @remove-run="removeRun"
            @clear-history="clearRuns"
          />
        </div>
      </section>

      <section v-else class="client-main client-empty">
        <p class="box-placeholder">
          Add a request to start. Paste a curl command into its URL box to fill it.
        </p>
      </section>
    </div>
  </div>
</template>

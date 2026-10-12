<script setup lang="ts">
import { computed, onMounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import { collidingUserName, userHandle } from '../../userHandle'
import type { VaultDomain, VaultDomainUser } from '../../../../preload/types'

const props = defineProps<{
  visible: boolean
}>()

const vaultUnlocked = toRef(vaultSession, 'unlocked')
const sessionPassphrase = toRef(vaultSession, 'passphrase')

const domains = ref<string[]>([])
const domainsLoading = ref(false)
const domainsError = ref('')

async function refreshDomains(): Promise<void> {
  domainsLoading.value = true
  domainsError.value = ''
  try {
    domains.value = await window.api.vault.listDomains()
  } catch (err) {
    domainsError.value = describeError(err)
  } finally {
    domainsLoading.value = false
  }
}

const deletingDomainName = ref<string | null>(null)

async function deleteDomain(name: string): Promise<void> {
  deletingDomainName.value = name
  try {
    await window.api.vault.deleteDomain(name)
    await refreshDomains()
  } catch (err) {
    domainsError.value = describeError(err)
  } finally {
    deletingDomainName.value = null
  }
}

type TtlMode = 'field' | 'jwt' | 'fixed'
type UserTestStatus = 'idle' | 'testing' | 'ok' | 'error'

interface UserFieldDraft {
  key: string
  value: string
}

interface UserDraft {
  id: number
  expanded: boolean
  name: string
  isByot: boolean
  token: string
  tags: string
  fields: UserFieldDraft[]
  testStatus: UserTestStatus
  testError: string
}

const showDomainForm = ref(false)
const editingDomainName = ref<string | null>(null)
const copiedFrom = ref('')
const loadingDomainName = ref<string | null>(null)
const newDomainName = ref('')
const newDomainBaseUrl = ref('')
const newDomainLoginEndpoint = ref('')
const newDomainTokenPath = ref('')
const newDomainTtlMode = ref<TtlMode>('fixed')
const newDomainTtlField = ref('')
const newDomainFixedTtlMinutes = ref(60)
const newDomainTokenKind = ref<'header' | 'cookie' | 'query'>('header')
const newDomainTokenName = ref('Authorization')
const newDomainTokenPrefix = ref('Bearer ')
const newDomainUsers = ref<UserDraft[]>([])
const domainFormError = ref('')
const creatingDomain = ref(false)
const settingsOpen = ref(true)
const userSearch = ref('')
let nextUserDraftId = 0

function openDomainForm(): void {
  editingDomainName.value = null
  copiedFrom.value = ''
  newDomainName.value = ''
  newDomainBaseUrl.value = ''
  newDomainLoginEndpoint.value = ''
  newDomainTokenPath.value = ''
  newDomainTtlMode.value = 'fixed'
  newDomainTtlField.value = ''
  newDomainFixedTtlMinutes.value = 60
  newDomainTokenKind.value = 'header'
  newDomainTokenName.value = 'Authorization'
  newDomainTokenPrefix.value = 'Bearer '
  newDomainUsers.value = []
  domainFormError.value = ''
  settingsOpen.value = true
  userSearch.value = ''
  showDomainForm.value = true
}

function cancelDomainForm(): void {
  showDomainForm.value = false
}

function userDraftFromDomainUser(user: VaultDomainUser): UserDraft {
  const fields = Object.entries(user.Fields ?? {}).map(([key, value]) => ({ key, value }))
  return {
    id: nextUserDraftId++,
    expanded: false,
    name: user.Name,
    isByot: user.Token !== '',
    token: user.Token,
    tags: (user.Tags ?? []).join(', '),
    fields: fields.length > 0 ? fields : [{ key: '', value: '' }],
    testStatus: 'idle',
    testError: ''
  }
}

function fillDomainForm(domain: VaultDomain): void {
  newDomainBaseUrl.value = domain.BaseURL
  newDomainLoginEndpoint.value = domain.LoginEndpoint
  newDomainTokenPath.value = domain.TokenPath
  if (domain.TTLOrigin === 'jwt-exp') {
    newDomainTtlMode.value = 'jwt'
    newDomainTtlField.value = ''
  } else if (domain.TTLOrigin !== '') {
    newDomainTtlMode.value = 'field'
    newDomainTtlField.value = domain.TTLOrigin
  } else {
    newDomainTtlMode.value = 'fixed'
    newDomainTtlField.value = ''
  }
  newDomainFixedTtlMinutes.value = domain.FixedTTLSeconds / 60
  newDomainTokenKind.value = domain.TokenPlacement.Kind
  newDomainTokenName.value = domain.TokenPlacement.Name
  newDomainTokenPrefix.value = domain.TokenPlacement.Prefix
  newDomainUsers.value = (domain.Users ?? []).map(userDraftFromDomainUser)
  domainFormError.value = ''
  userSearch.value = ''
  showDomainForm.value = true
}

async function openDomainEdit(name: string): Promise<void> {
  if (!vaultUnlocked.value || loadingDomainName.value) return
  loadingDomainName.value = name
  domainsError.value = ''
  try {
    const domain = await window.api.vault.getDomain(sessionPassphrase.value, name)
    editingDomainName.value = name
    copiedFrom.value = ''
    newDomainName.value = name
    fillDomainForm(domain)
    settingsOpen.value = false
  } catch (err) {
    domainsError.value = describeError(err)
  } finally {
    loadingDomainName.value = null
  }
}

async function openDomainCopy(name: string): Promise<void> {
  if (!vaultUnlocked.value || loadingDomainName.value) return
  loadingDomainName.value = name
  domainsError.value = ''
  try {
    const domain = await window.api.vault.getDomain(sessionPassphrase.value, name)
    editingDomainName.value = null
    copiedFrom.value = name
    newDomainName.value = `${name} copy`
    fillDomainForm(domain)
    settingsOpen.value = true
  } catch (err) {
    domainsError.value = describeError(err)
  } finally {
    loadingDomainName.value = null
  }
}

function addUserDraft(): void {
  userSearch.value = ''
  newDomainUsers.value.unshift({
    id: nextUserDraftId++,
    expanded: true,
    name: '',
    isByot: false,
    token: '',
    tags: '',
    fields: [{ key: '', value: '' }],
    testStatus: 'idle',
    testError: ''
  })
}

function removeUserDraft(index: number): void {
  newDomainUsers.value.splice(index, 1)
}

function addUserField(userIndex: number): void {
  newDomainUsers.value[userIndex].fields.push({ key: '', value: '' })
}

function removeUserField(userIndex: number, fieldIndex: number): void {
  newDomainUsers.value[userIndex].fields.splice(fieldIndex, 1)
}

// Builds a plain Domain object with no users, since IPC structured clone cannot clone a Vue reactive Proxy.
function buildDomainDraft(): VaultDomain {
  const ttlOrigin =
    newDomainTtlMode.value === 'jwt'
      ? 'jwt-exp'
      : newDomainTtlMode.value === 'field'
        ? newDomainTtlField.value.trim()
        : ''
  return {
    BaseURL: newDomainBaseUrl.value.trim(),
    LoginEndpoint: newDomainLoginEndpoint.value.trim(),
    TokenPath: newDomainTokenPath.value.trim(),
    TTLOrigin: ttlOrigin,
    FixedTTLSeconds: Math.round(newDomainFixedTtlMinutes.value * 60),
    TokenPlacement: {
      Kind: newDomainTokenKind.value,
      Name: newDomainTokenName.value.trim(),
      Prefix: newDomainTokenPrefix.value
    },
    Users: []
  }
}

function buildUserPayload(draft: UserDraft): VaultDomainUser {
  const tags = userTagList(draft)
  if (draft.isByot) {
    return { Name: draft.name.trim(), Fields: {}, Token: draft.token, Tags: tags }
  }
  const fields: Record<string, string> = {}
  for (const f of draft.fields) {
    if (f.key.trim()) fields[f.key.trim()] = f.value
  }
  return { Name: draft.name.trim(), Fields: fields, Token: '', Tags: tags }
}

async function testUserDraft(index: number): Promise<void> {
  const draft = newDomainUsers.value[index]
  draft.testStatus = 'testing'
  draft.testError = ''
  try {
    await window.api.vault.testDomainLogin(buildDomainDraft(), buildUserPayload(draft))
    draft.testStatus = 'ok'
  } catch (err) {
    draft.testStatus = 'error'
    draft.testError = describeError(err)
  }
}

const domainNameTaken = computed(
  () => editingDomainName.value === null && domains.value.includes(newDomainName.value.trim())
)

const userNames = computed(() => newDomainUsers.value.map((u) => u.name))

function userTagList(draft: UserDraft): string[] {
  return draft.tags
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
}

const visibleUsers = computed(() => {
  const query = userSearch.value.trim().toLowerCase()
  return newDomainUsers.value
    .map((user, index) => ({ user, index }))
    .filter(
      ({ user }) =>
        query === '' ||
        user.name.toLowerCase().includes(query) ||
        userHandle(user.name).includes(query) ||
        user.tags.toLowerCase().includes(query)
    )
})

// A user whose problem would be hidden by collapsing stays open.
function userEditorOpen(draft: UserDraft, index: number): boolean {
  return (
    draft.expanded ||
    draft.name.trim() === '' ||
    draft.testStatus === 'error' ||
    collidingUserName(userNames.value, index) !== null
  )
}

const canSubmitDomain = computed(
  () =>
    newDomainName.value.trim().length > 0 &&
    !domainNameTaken.value &&
    newDomainBaseUrl.value.trim().length > 0 &&
    newDomainUsers.value.length > 0 &&
    newDomainUsers.value.every((u) => u.name.trim().length > 0)
)

async function submitDomain(): Promise<void> {
  if (!canSubmitDomain.value || !vaultUnlocked.value || creatingDomain.value) return
  creatingDomain.value = true
  domainFormError.value = ''
  try {
    const domain = buildDomainDraft()
    domain.Users = newDomainUsers.value.map(buildUserPayload)
    if (editingDomainName.value !== null) {
      await window.api.vault.updateDomain(sessionPassphrase.value, editingDomainName.value, domain)
    } else {
      await window.api.vault.putDomain(sessionPassphrase.value, newDomainName.value.trim(), domain)
    }
    showDomainForm.value = false
    await refreshDomains()
  } catch (err) {
    domainFormError.value = describeError(err)
  } finally {
    creatingDomain.value = false
  }
}

onMounted(refreshDomains)

watch(
  () => props.visible,
  (visible) => {
    if (visible) refreshDomains()
  }
)
</script>

<template>
  <div class="section">
    <div class="section-toolbar">
      <p class="hint">
        Saved HTTP hosts with their login and users. HTTP channels are created from a domain.
      </p>
      <button
        v-if="!showDomainForm"
        class="primary-button new-channel-button"
        @click="openDomainForm"
      >
        + New domain
      </button>
    </div>

    <div v-if="showDomainForm" class="inline-section">
      <h4 class="inline-section-title">
        {{
          editingDomainName !== null
            ? 'Edit domain'
            : copiedFrom
              ? `New domain from ${copiedFrom}`
              : 'New domain'
        }}
      </h4>

      <div class="field-block">
        <label class="field-label" for="domain-name">Name</label>
        <input
          id="domain-name"
          v-model="newDomainName"
          type="text"
          placeholder="e.g. staging-api"
          :disabled="editingDomainName !== null"
          autofocus
        />
        <p v-if="domainNameTaken" class="error-text">A domain with this name already exists.</p>
      </div>

      <button
        type="button"
        class="settings-toggle"
        :aria-expanded="settingsOpen"
        @click="settingsOpen = !settingsOpen"
      >
        <span class="settings-chevron" :class="{ expanded: settingsOpen }">▸</span>
        Settings
        <span v-if="!settingsOpen" class="settings-summary">{{ newDomainBaseUrl }}</span>
      </button>

      <div v-show="settingsOpen" class="settings-body">
        <div class="field-block">
          <label class="field-label" for="domain-base-url">Base URL</label>
          <input
            id="domain-base-url"
            v-model="newDomainBaseUrl"
            type="text"
            placeholder="https://api.example.com"
          />
        </div>

        <div class="field-block">
          <label class="field-label" for="domain-login-endpoint">Login endpoint</label>
          <input
            id="domain-login-endpoint"
            v-model="newDomainLoginEndpoint"
            type="text"
            placeholder="/auth/login (leave blank if every user is bring-your-own-token)"
          />
        </div>

        <div class="field-block">
          <label class="field-label" for="domain-token-path">Token path</label>
          <input
            id="domain-token-path"
            v-model="newDomainTokenPath"
            type="text"
            placeholder="e.g. access_token or data.token"
          />
          <p class="hint">
            Where the access token lives in the login response, as a dotted JSON path.
          </p>
        </div>

        <div class="field-block">
          <label class="field-label">Token lifetime (TTLOrigin)</label>
          <div class="radio-row">
            <label class="radio-option">
              <input v-model="newDomainTtlMode" type="radio" value="fixed" />
              Fixed
            </label>
            <label class="radio-option">
              <input v-model="newDomainTtlMode" type="radio" value="field" />
              From response field
            </label>
            <label class="radio-option">
              <input v-model="newDomainTtlMode" type="radio" value="jwt" />
              Decode as JWT (jwt-exp)
            </label>
          </div>
          <input
            v-if="newDomainTtlMode === 'fixed'"
            v-model.number="newDomainFixedTtlMinutes"
            type="number"
            min="1"
            title="Fixed TTL, minutes"
          />
          <input
            v-if="newDomainTtlMode === 'field'"
            v-model="newDomainTtlField"
            type="text"
            placeholder="e.g. expires_in"
          />
        </div>

        <div class="field-block">
          <label class="field-label">Token placement</label>
          <div class="field-grid">
            <div>
              <select v-model="newDomainTokenKind">
                <option value="header">Header</option>
                <option value="cookie">Cookie</option>
                <option value="query">Query param</option>
              </select>
            </div>
            <div>
              <input v-model="newDomainTokenName" type="text" placeholder="e.g. Authorization" />
            </div>
          </div>
          <input
            v-if="newDomainTokenKind === 'header'"
            v-model="newDomainTokenPrefix"
            type="text"
            placeholder="Prefix, e.g. 'Bearer '"
          />
        </div>
      </div>

      <div class="field-block">
        <div class="users-toolbar">
          <label class="field-label">Users ({{ newDomainUsers.length }})</label>
          <input
            v-model="userSearch"
            type="search"
            class="users-search"
            placeholder="Search name or tag"
          />
          <button type="button" class="ghost-button" @click="addUserDraft">+ User</button>
        </div>
        <p v-if="newDomainUsers.length > 0 && visibleUsers.length === 0" class="empty">
          No user matches.
        </p>
        <div v-for="{ user, index: userIndex } in visibleUsers" :key="user.id" class="user-draft">
          <button
            type="button"
            class="user-summary"
            :aria-expanded="userEditorOpen(user, userIndex)"
            @click="user.expanded = !user.expanded"
          >
            <span class="settings-chevron" :class="{ expanded: userEditorOpen(user, userIndex) }"
              >▸</span
            >
            <span class="user-summary-name">{{ user.name.trim() || 'New user' }}</span>
            <span class="capsule-row">
              <span v-for="tag in userTagList(user)" :key="tag" class="value-capsule">{{
                tag
              }}</span>
            </span>
            <span class="user-summary-kind">{{ user.isByot ? 'Token' : 'Login' }}</span>
            <span
              class="status-capsule"
              :class="{
                'status-ok': user.testStatus === 'ok',
                'status-error': user.testStatus === 'error'
              }"
            >
              <template v-if="user.testStatus === 'ok'">OK</template>
              <template v-else-if="user.testStatus === 'error'">Failed</template>
              <template v-else-if="user.testStatus === 'testing'">Testing</template>
              <template v-else>Not tested</template>
            </span>
          </button>

          <template v-if="userEditorOpen(user, userIndex)">
            <div class="user-name-row">
              <input v-model="user.name" type="text" placeholder="User name" />
              <label class="radio-option">
                <input v-model="user.isByot" type="checkbox" />
                Bring your own token
              </label>
            </div>

            <p
              v-if="userHandle(user.name) !== '' && userHandle(user.name) !== user.name"
              class="hint"
            >
              Models use this user as {{ userHandle(user.name) }}.
            </p>
            <p v-if="collidingUserName(userNames, userIndex) !== null" class="error-text">
              Models would use the same handle, {{ userHandle(user.name) }}, for "{{
                collidingUserName(userNames, userIndex)
              }}". Rename one of them.
            </p>

            <input
              v-model="user.tags"
              type="text"
              placeholder="Tags, comma separated, e.g. tenant x, low level"
            />

            <input
              v-if="user.isByot"
              v-model="user.token"
              type="text"
              placeholder="Paste the token"
            />

            <div v-else class="kv-list">
              <div v-for="(field, fieldIndex) in user.fields" :key="fieldIndex" class="kv-row">
                <input v-model="field.key" type="text" placeholder="field name" />
                <input v-model="field.value" type="text" placeholder="value" />
                <button
                  type="button"
                  class="chip-remove"
                  title="Remove field"
                  @click="removeUserField(userIndex, fieldIndex)"
                >
                  ×
                </button>
              </div>
              <button type="button" class="ghost-button" @click="addUserField(userIndex)">
                + Field
              </button>
            </div>

            <div class="test-row">
              <button
                class="ghost-button"
                :disabled="user.testStatus === 'testing'"
                @click="testUserDraft(userIndex)"
              >
                {{ user.testStatus === 'testing' ? 'Testing...' : 'Test login' }}
              </button>
              <span
                class="test-status"
                :class="{
                  'test-status-ok': user.testStatus === 'ok',
                  'test-status-error': user.testStatus === 'error'
                }"
              >
                <template v-if="user.testStatus === 'ok'">✓ OK</template>
                <template v-else-if="user.testStatus === 'error'">✗ {{ user.testError }}</template>
                <template v-else-if="user.testStatus === 'testing'">Testing...</template>
                <template v-else>Not tested yet</template>
              </span>
              <button type="button" class="ghost-button danger" @click="removeUserDraft(userIndex)">
                Remove user
              </button>
            </div>
          </template>
        </div>
      </div>

      <div class="create-actions form-action-bar">
        <p v-if="!vaultUnlocked" class="hint">Unlock the vault above before saving.</p>
        <p v-if="domainFormError" class="error-text">{{ domainFormError }}</p>
        <button class="ghost-button" @click="cancelDomainForm">Cancel</button>
        <button
          class="primary-button"
          :disabled="!canSubmitDomain || creatingDomain"
          @click="submitDomain"
        >
          {{ creatingDomain ? 'Saving...' : 'Save' }}
        </button>
      </div>
    </div>

    <p v-if="domainsError" class="error-text">{{ domainsError }}</p>
    <p v-else-if="domainsLoading" class="empty">Loading...</p>
    <p v-else-if="domains.length === 0" class="empty">No domains yet.</p>

    <div v-else class="channel-list">
      <div v-for="name in domains" :key="name" class="channel-row">
        <div class="channel-info">
          <span class="channel-db">{{ name }}</span>
        </div>
        <div class="channel-actions">
          <button
            class="ghost-button"
            :disabled="!vaultUnlocked || loadingDomainName === name"
            :title="!vaultUnlocked ? 'Unlock the vault first' : 'Create a new domain from this one'"
            @click="openDomainCopy(name)"
          >
            Copy
          </button>
          <button
            class="ghost-button"
            :disabled="!vaultUnlocked || loadingDomainName === name"
            :title="!vaultUnlocked ? 'Unlock the vault first' : ''"
            @click="openDomainEdit(name)"
          >
            {{ loadingDomainName === name ? 'Loading...' : 'Edit' }}
          </button>
          <button
            class="ghost-button danger"
            :disabled="deletingDomainName === name"
            @click="deleteDomain(name)"
          >
            {{ deletingDomainName === name ? 'Deleting...' : 'Delete' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

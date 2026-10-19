<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef, watch } from 'vue'
import { vaultSession, describeError, unlockVaultSession } from '../../vaultSession'
import type {
  Project,
  VaultPolicies,
  VaultPolicy,
  VaultPolicyRule
} from '../../../../preload/types'
import AppButton from '../AppButton.vue'

const props = defineProps<{
  projects: Project[]
  activeProjectId: string | null
}>()

type Mode = 'allow' | 'deny' | 'approval'

const GLOBAL = 'global'

const CATEGORY_ORDER = [
  'Deletion',
  'Git',
  'System',
  'Network',
  'Database',
  'Infrastructure',
  'Lemongrass',
  'Other'
]

const vaultUnlocked = toRef(vaultSession, 'unlocked')

const catalog = ref<VaultPolicyRule[]>([])
const policies = ref<VaultPolicies>({ global: emptyPolicy(), projects: {} })
const scope = ref<string>(GLOBAL)

const modes = ref<Record<string, Mode>>({})
const binaries = ref<string[]>([])
const paths = ref<string[]>([])
const excludeBinaries = ref<string[]>([])
const excludePaths = ref<string[]>([])
const savedSnapshot = ref('')

const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
const savedNotice = ref('')
const passphraseInput = ref('')
const confirmingReset = ref(false)

const newBinary = ref('')
const newPath = ref('')
const binaryError = ref('')
const pathError = ref('')

function emptyPolicy(): VaultPolicy {
  return { rules: {}, binaries: [], paths: [], exclude_binaries: [], exclude_paths: [] }
}

const isGlobal = computed(() => scope.value === GLOBAL)
const globalBinaries = computed(() => (isGlobal.value ? [] : policies.value.global.binaries))
const globalPaths = computed(() => (isGlobal.value ? [] : policies.value.global.paths))

function sameBinary(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase()
}

const groups = computed(() => {
  const byCategory = new Map<string, VaultPolicyRule[]>()
  for (const rule of catalog.value) {
    const list = byCategory.get(rule.category) ?? []
    list.push(rule)
    byCategory.set(rule.category, list)
  }
  const ordered = CATEGORY_ORDER.filter((name) => byCategory.has(name))
  const extra = [...byCategory.keys()].filter((name) => !CATEGORY_ORDER.includes(name))
  return [...ordered, ...extra].map((name) => ({ name, rules: byCategory.get(name) ?? [] }))
})

const filterOpen = ref(false)
const filterQuery = ref('')
const filterInput = ref<HTMLInputElement | null>(null)

// Filtering only hides rows, so a change in a hidden rule still counts and is saved.
const visibleGroups = computed(() => {
  const query = filterQuery.value.trim().toLowerCase()
  if (!query) return groups.value
  return groups.value
    .map((group) => ({
      name: group.name,
      rules: group.rules.filter((rule) =>
        [rule.description, rule.id, rule.category].some((text) =>
          text.toLowerCase().includes(query)
        )
      )
    }))
    .filter((group) => group.rules.length > 0)
})
const visibleRuleCount = computed(() =>
  visibleGroups.value.reduce((total, group) => total + group.rules.length, 0)
)

async function openFilter(): Promise<void> {
  filterOpen.value = true
  await nextTick()
  filterInput.value?.focus()
  filterInput.value?.select()
}

function closeFilter(): void {
  filterOpen.value = false
  filterQuery.value = ''
}

function onKeydown(event: KeyboardEvent): void {
  if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'f') {
    event.preventDefault()
    event.stopPropagation()
    void openFilter()
  }
}

function onFilterKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    closeFilter()
  }
}

// The global policy holds custom rules only. A project's exclusions keep only entries global still has, so a stale one is dropped on save.
function buildPolicy(): VaultPolicy {
  if (isGlobal.value) {
    return { ...emptyPolicy(), binaries: [...binaries.value], paths: [...paths.value] }
  }
  const rules: VaultPolicy['rules'] = {}
  for (const rule of catalog.value) {
    if (rule.locked) continue
    const chosen = modes.value[rule.id]
    if (chosen && chosen !== rule.mode) rules[rule.id] = chosen
  }
  return {
    rules,
    binaries: [...binaries.value],
    paths: [...paths.value],
    exclude_binaries: excludeBinaries.value.filter((name) =>
      globalBinaries.value.some((g) => sameBinary(g, name))
    ),
    exclude_paths: excludePaths.value.filter((value) => globalPaths.value.includes(value))
  }
}

// Text typed into an add field but not yet added counts as a change, and Save adds it first.
const pendingInput = computed(() => newBinary.value.trim() !== '' || newPath.value.trim() !== '')
const dirty = computed(
  () => pendingInput.value || JSON.stringify(buildPolicy()) !== savedSnapshot.value
)
const passphrase = computed(() =>
  vaultUnlocked.value ? vaultSession.passphrase : passphraseInput.value
)

function applyPolicy(policy: VaultPolicy): void {
  const next: Record<string, Mode> = {}
  for (const rule of catalog.value) {
    next[rule.id] = policy.rules[rule.id] ?? rule.mode
  }
  modes.value = next
  binaries.value = [...policy.binaries]
  paths.value = [...policy.paths]
  excludeBinaries.value = [...policy.exclude_binaries]
  excludePaths.value = [...policy.exclude_paths]
}

// Switching scope drops unsaved changes, typed add fields included.
function showScope(): void {
  newBinary.value = ''
  newPath.value = ''
  binaryError.value = ''
  pathError.value = ''
  savedNotice.value = ''
  saveError.value = ''
  confirmingReset.value = false
  applyPolicy(
    isGlobal.value ? policies.value.global : (policies.value.projects[scope.value] ?? emptyPolicy())
  )
  savedSnapshot.value = JSON.stringify(buildPolicy())
}

async function load(): Promise<void> {
  if (loading.value) return
  loading.value = true
  loadError.value = ''
  try {
    catalog.value = await window.api.vault.policyCatalog()
    policies.value = await window.api.vault.listPolicies()
    const active = props.activeProjectId
    const initial = active !== null && props.projects.some((p) => p.id === active) ? active : GLOBAL
    if (scope.value === initial) showScope()
    else scope.value = initial
  } catch (err) {
    loadError.value = describeError(err)
  } finally {
    loading.value = false
  }
}

// A passphrase typed here unlocks the vault for the whole app, the same as unlocking the Vault tab.
function rememberPassphrase(): void {
  if (!vaultUnlocked.value) {
    unlockVaultSession(passphraseInput.value)
    passphraseInput.value = ''
  }
}

async function save(): Promise<void> {
  if (saving.value || !dirty.value || !passphrase.value) return
  if (newBinary.value.trim()) addBinary()
  if (newPath.value.trim()) addPath()
  if (binaryError.value || pathError.value) return
  saving.value = true
  saveError.value = ''
  savedNotice.value = ''
  try {
    const policy = buildPolicy()
    await window.api.vault.putPolicy(passphrase.value, isGlobal.value ? null : scope.value, policy)
    rememberPassphrase()
    policies.value = await window.api.vault.listPolicies()
    savedSnapshot.value = JSON.stringify(policy)
    savedNotice.value = 'Saved. Running agent tabs use it from their next tool call.'
  } catch (err) {
    saveError.value = describeError(err)
  } finally {
    saving.value = false
  }
}

// The first click arms the reset and the second clears the shown scope, which still needs Save.
function resetToDefaults(): void {
  if (!confirmingReset.value) {
    confirmingReset.value = true
    return
  }
  confirmingReset.value = false
  applyPolicy(emptyPolicy())
  savedNotice.value = ''
}

function addBinary(): void {
  const name = newBinary.value.trim()
  binaryError.value = ''
  if (!name) return
  if (/[\s/]/.test(name)) {
    binaryError.value = 'Use the binary name only, without spaces or a path.'
    return
  }
  if (name.toLowerCase() === 'lgrass') {
    binaryError.value = 'lgrass cannot be blocked. The connectors depend on it.'
    return
  }
  if (globalBinaries.value.some((g) => sameBinary(g, name))) {
    binaryError.value = 'The global policy already blocks it.'
    return
  }
  if (!binaries.value.some((existing) => sameBinary(existing, name))) {
    binaries.value.push(name)
  }
  newBinary.value = ''
}

function addPath(): void {
  const value = newPath.value.trim()
  pathError.value = ''
  if (!value) return
  if (value !== '~' && !value.startsWith('~/') && !value.startsWith('/')) {
    pathError.value = 'Start the path with / or ~/.'
    return
  }
  if (globalPaths.value.includes(value)) {
    pathError.value = 'The global policy already blocks it.'
    return
  }
  if (!paths.value.includes(value)) paths.value.push(value)
  newPath.value = ''
}

function removeBinary(name: string): void {
  binaries.value = binaries.value.filter((existing) => existing !== name)
}

function removePath(value: string): void {
  paths.value = paths.value.filter((existing) => existing !== value)
}

function binaryExcluded(name: string): boolean {
  return excludeBinaries.value.some((ex) => sameBinary(ex, name))
}

function toggleBinaryExclude(name: string): void {
  excludeBinaries.value = binaryExcluded(name)
    ? excludeBinaries.value.filter((ex) => !sameBinary(ex, name))
    : [...excludeBinaries.value, name]
}

function pathExcluded(value: string): boolean {
  return excludePaths.value.includes(value)
}

function togglePathExclude(value: string): void {
  excludePaths.value = pathExcluded(value)
    ? excludePaths.value.filter((ex) => ex !== value)
    : [...excludePaths.value, value]
}

watch(scope, showScope)

onMounted(() => {
  window.addEventListener('keydown', onKeydown, { capture: true })
  void load()
})

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown, { capture: true }))
</script>

<template>
  <div class="section">
    <div class="section-head">
      <p class="hint">
        What agents may run in a lemongrass tab. Agents cannot change it: saving needs your vault
        passphrase. Rules marked Locked protect lemongrass itself.
      </p>

      <div class="scope-row">
        <label class="scope-label" for="policy-scope">Policy for</label>
        <select id="policy-scope" v-model="scope" class="scope-select">
          <option :value="GLOBAL">Global</option>
          <option v-for="project in props.projects" :key="project.id" :value="project.id">
            {{ project.name }}
          </option>
        </select>
        <AppButton
          :variant="confirmingReset ? 'danger' : 'ghost'"
          :disabled="loading || saving"
          @click="resetToDefaults"
          @mouseleave="confirmingReset = false"
        >
          {{ confirmingReset ? 'Click again to reset' : 'Reset to defaults' }}
        </AppButton>
      </div>
      <div v-if="filterOpen" class="find">
        <input
          ref="filterInput"
          v-model="filterQuery"
          class="find-input"
          placeholder="Filter rules"
          spellcheck="false"
          aria-label="Filter rules"
          @keydown="onFilterKeydown"
        />
        <span v-if="!isGlobal && filterQuery.trim()" class="find-count">
          {{ visibleRuleCount ? `${visibleRuleCount} rules` : 'No rules match' }}
        </span>
        <button class="find-close" title="Close (Esc)" @click="closeFilter">&times;</button>
      </div>
      <p v-if="isGlobal" class="hint">
        Global blocks apply to every project. A project can exclude any of them.
      </p>
      <p v-else class="hint">
        Rule modes are this project's own. Global blocks apply here unless excluded.
      </p>

      <p v-if="loadError" class="error-text">{{ loadError }}</p>
      <p v-else-if="loading && !catalog.length" class="empty">Loading the policy...</p>
    </div>

    <div v-if="catalog.length" class="policy-scroll lg-scroll">
      <template v-if="!isGlobal">
        <p class="hint">
          Needs approval is refused for now. Approving a call with your passphrase is not built yet.
        </p>

        <div v-for="group in visibleGroups" :key="group.name" class="policy-group">
          <h4 class="inline-section-title">{{ group.name }}</h4>
          <div class="rule-list">
            <div v-for="rule in group.rules" :key="rule.id" class="rule-row">
              <div class="rule-info">
                <span class="rule-name">{{ rule.description }}</span>
                <span class="rule-id">{{ rule.id }}</span>
              </div>
              <span v-if="rule.locked" class="value-capsule">Locked</span>
              <select v-else v-model="modes[rule.id]" class="policy-mode" :aria-label="rule.id">
                <option value="deny">Deny</option>
                <option value="approval">Needs approval</option>
                <option value="allow">Allow</option>
              </select>
            </div>
          </div>
        </div>
      </template>

      <div class="policy-group">
        <h4 class="inline-section-title">Blocked binaries</h4>
        <p class="hint">
          Agents cannot run these commands, wherever they appear in a shell line. lgrass cannot be
          added.
        </p>
        <div class="add-row">
          <input
            v-model="newBinary"
            type="text"
            placeholder="e.g. make"
            @keydown.enter="addBinary"
          />
          <AppButton :disabled="!newBinary.trim()" @click="addBinary">Add</AppButton>
        </div>
        <p v-if="binaryError" class="error-text">{{ binaryError }}</p>
        <div v-if="globalBinaries.length || binaries.length" class="chip-row">
          <span
            v-for="name in globalBinaries"
            :key="`global-${name}`"
            class="chip chip--global"
            :class="{ 'chip--excluded': binaryExcluded(name) }"
          >
            <span class="chip-text">{{ name }}</span>
            <span class="chip-tag">global</span>
            <button class="chip-action" @click="toggleBinaryExclude(name)">
              {{ binaryExcluded(name) ? 'Include' : 'Exclude' }}
            </button>
          </span>
          <span v-for="name in binaries" :key="name" class="chip">
            {{ name }}
            <button class="chip-remove" :aria-label="`Remove ${name}`" @click="removeBinary(name)">
              &times;
            </button>
          </span>
        </div>
      </div>

      <div class="policy-group">
        <h4 class="inline-section-title">Blocked paths</h4>
        <p class="hint">
          No tool may read or write these. A folder covers everything inside it, and * matches part
          of a name, for example ~/private or /data/*/secret.
        </p>
        <div class="add-row">
          <input
            v-model="newPath"
            type="text"
            placeholder="e.g. ~/private"
            @keydown.enter="addPath"
          />
          <AppButton :disabled="!newPath.trim()" @click="addPath">Add</AppButton>
        </div>
        <p v-if="pathError" class="error-text">{{ pathError }}</p>
        <div v-if="globalPaths.length || paths.length" class="chip-row">
          <span
            v-for="value in globalPaths"
            :key="`global-${value}`"
            class="chip chip--global"
            :class="{ 'chip--excluded': pathExcluded(value) }"
          >
            <span class="chip-text">{{ value }}</span>
            <span class="chip-tag">global</span>
            <button class="chip-action" @click="togglePathExclude(value)">
              {{ pathExcluded(value) ? 'Include' : 'Exclude' }}
            </button>
          </span>
          <span v-for="value in paths" :key="value" class="chip">
            {{ value }}
            <button class="chip-remove" :aria-label="`Remove ${value}`" @click="removePath(value)">
              &times;
            </button>
          </span>
        </div>
      </div>
    </div>

    <div v-if="catalog.length" class="save-bar">
      <span v-if="saveError" class="error-text bar-status">{{ saveError }}</span>
      <span v-else-if="dirty" class="bar-status">Unsaved changes</span>
      <span v-else-if="savedNotice" class="saved-text bar-status">{{ savedNotice }}</span>
      <span v-else class="bar-status muted">No changes</span>
      <input
        v-if="!vaultUnlocked && dirty"
        v-model="passphraseInput"
        type="password"
        class="passphrase-input"
        placeholder="Vault passphrase"
        aria-label="Vault passphrase"
        @keydown.enter="save"
      />
      <AppButton :disabled="!dirty || saving" @click="showScope">Discard</AppButton>
      <AppButton variant="primary" :disabled="!dirty || saving || !passphrase" @click="save">
        {{ saving ? 'Saving...' : 'Save policy' }}
      </AppButton>
    </div>
  </div>
</template>

<style scoped>
.section {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.section-head {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

/* Only the rules scroll, so the picker and the save bar stay inside the modal's padding. */
.policy-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-2) var(--space-3) 0;
  border-top: 1px solid var(--color-border-default);
}

.hint {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  line-height: 1.4;
}

.scope-row,
.add-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.save-bar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border-default);
}

.bar-status {
  flex: 1;
  min-width: 0;
  color: var(--color-fg-primary);
  font-size: var(--text-xs);
}

.bar-status.muted {
  color: var(--color-fg-muted);
}

.scope-label {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.scope-select {
  flex: 1;
  min-width: 0;
}

.add-row input {
  flex: 1;
  min-width: 0;
}

.save-bar .passphrase-input {
  width: 12rem;
}

input,
select {
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
}

select {
  /* The native chevron sits in a fixed inset that padding does not reliably move, so it is dropped. */
  appearance: none;
}

input:focus,
select:focus {
  outline: none;
  border-color: var(--color-amber);
}

.find {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.find .find-input {
  flex: 1;
  min-width: 0;
}

.find-count {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-variant-numeric: tabular-nums;
}

.find-close {
  padding: 0 var(--space-1);
  background: transparent;
  border: none;
  color: var(--color-fg-muted);
  font-size: var(--text-md);
  line-height: 1;
  cursor: pointer;
}

.find-close:hover {
  color: var(--color-fg-primary);
}

.policy-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.inline-section-title {
  font-family: var(--font-display);
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
  color: var(--color-fg-primary);
}

.rule-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.rule-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-3);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
}

.rule-info {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
}

.rule-name {
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
}

.rule-id {
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  color: var(--color-fg-muted);
}

.policy-mode {
  flex-shrink: 0;
  min-width: 10rem;
  background: var(--color-surface-1);
}

.value-capsule {
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-surface-3);
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px var(--space-2);
  background: var(--color-surface-2);
  border-radius: var(--radius-pill);
  color: var(--color-fg-primary);
  font-size: var(--text-xs);
}

.chip--global {
  background: var(--color-surface-3);
}

.chip--excluded .chip-text {
  color: var(--color-fg-muted);
  text-decoration: line-through;
}

.chip-tag {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.chip-action {
  padding: 0;
  background: transparent;
  border: none;
  color: var(--color-amber);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  cursor: pointer;
}

.chip-action:hover {
  text-decoration: underline;
}

.chip-remove {
  background: transparent;
  border: none;
  color: var(--color-fg-muted);
  cursor: pointer;
  font-size: var(--text-sm);
  line-height: 1;
  padding: 0;
}

.chip-remove:hover {
  color: var(--color-fg-primary);
}

.empty {
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}

.saved-text {
  color: var(--color-success);
  font-size: var(--text-xs);
}

.error-text {
  color: var(--color-danger, #e5484d);
  font-size: var(--text-xs);
}
</style>

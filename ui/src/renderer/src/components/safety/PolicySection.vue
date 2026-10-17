<script setup lang="ts">
import { computed, ref, toRef, watch } from 'vue'
import { vaultSession, describeError } from '../../vaultSession'
import type { VaultPolicy, VaultPolicyRule } from '../../../../preload/types'
import AppButton from '../AppButton.vue'

const props = defineProps<{
  visible: boolean
}>()

type Mode = 'allow' | 'deny' | 'approval'

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
const sessionPassphrase = toRef(vaultSession, 'passphrase')

const catalog = ref<VaultPolicyRule[]>([])
const modes = ref<Record<string, Mode>>({})
const binaries = ref<string[]>([])
const paths = ref<string[]>([])
const savedSnapshot = ref('')

const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
const savedNotice = ref('')

const newBinary = ref('')
const newPath = ref('')
const binaryError = ref('')
const pathError = ref('')

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

function buildPolicy(): VaultPolicy {
  const rules: VaultPolicy['rules'] = {}
  for (const rule of catalog.value) {
    if (rule.locked) continue
    const chosen = modes.value[rule.id]
    if (chosen && chosen !== rule.mode) rules[rule.id] = chosen
  }
  return { rules, binaries: [...binaries.value], paths: [...paths.value] }
}

const dirty = computed(() => JSON.stringify(buildPolicy()) !== savedSnapshot.value)

function applyPolicy(policy: VaultPolicy): void {
  const next: Record<string, Mode> = {}
  for (const rule of catalog.value) {
    next[rule.id] = policy.rules[rule.id] ?? rule.mode
  }
  modes.value = next
  binaries.value = [...policy.binaries]
  paths.value = [...policy.paths]
  savedSnapshot.value = JSON.stringify(buildPolicy())
}

async function load(): Promise<void> {
  if (!vaultUnlocked.value || loading.value) return
  loading.value = true
  loadError.value = ''
  try {
    catalog.value = await window.api.vault.policyCatalog()
    applyPolicy(await window.api.vault.getPolicy(sessionPassphrase.value))
  } catch (err) {
    loadError.value = describeError(err)
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  if (saving.value || !dirty.value) return
  saving.value = true
  saveError.value = ''
  savedNotice.value = ''
  try {
    const policy = buildPolicy()
    await window.api.vault.putPolicy(sessionPassphrase.value, policy)
    savedSnapshot.value = JSON.stringify(policy)
    savedNotice.value = 'Saved. Running agent tabs use it from their next tool call.'
  } catch (err) {
    saveError.value = describeError(err)
  } finally {
    saving.value = false
  }
}

function resetToDefaults(): void {
  applyPolicy({ rules: {}, binaries: [], paths: [] })
  savedSnapshot.value = ''
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
  if (!binaries.value.some((existing) => existing.toLowerCase() === name.toLowerCase())) {
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
  if (!paths.value.includes(value)) paths.value.push(value)
  newPath.value = ''
}

function removeBinary(name: string): void {
  binaries.value = binaries.value.filter((existing) => existing !== name)
}

function removePath(value: string): void {
  paths.value = paths.value.filter((existing) => existing !== value)
}

watch(
  () => [props.visible, vaultUnlocked.value],
  ([visible, unlocked]) => {
    if (visible && unlocked) load()
  },
  { immediate: true }
)
</script>

<template>
  <div class="section">
    <div class="section-toolbar">
      <p class="hint">
        What agents may run in a lemongrass tab. The policy is sealed in the vault, so an agent
        cannot change it. Rules marked Locked protect lemongrass itself.
      </p>
      <AppButton :disabled="loading || saving" @click="resetToDefaults">
        Reset to defaults
      </AppButton>
    </div>

    <p v-if="loadError" class="error-text">{{ loadError }}</p>
    <p v-else-if="loading && !catalog.length" class="empty">Loading the policy...</p>

    <template v-if="catalog.length">
      <p class="hint">
        Needs approval is refused for now. Approving a call with your passphrase is not built yet.
      </p>

      <div v-for="group in groups" :key="group.name" class="policy-group">
        <h4 class="inline-section-title">{{ group.name }}</h4>
        <div class="channel-list">
          <div v-for="rule in group.rules" :key="rule.id" class="channel-row">
            <div class="channel-info">
              <span class="channel-name">{{ rule.description }}</span>
              <span class="channel-db policy-rule-id">{{ rule.id }}</span>
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

      <div class="policy-group">
        <h4 class="inline-section-title">Blocked binaries</h4>
        <p class="hint">
          Agents cannot run these commands, wherever they appear in a shell line. lgrass cannot be
          added.
        </p>
        <div class="unlock-row">
          <input
            v-model="newBinary"
            type="text"
            placeholder="e.g. make"
            @keydown.enter="addBinary"
          />
          <AppButton :disabled="!newBinary.trim()" @click="addBinary">Add</AppButton>
        </div>
        <p v-if="binaryError" class="error-text">{{ binaryError }}</p>
        <div v-if="binaries.length" class="chip-row">
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
        <div class="unlock-row">
          <input
            v-model="newPath"
            type="text"
            placeholder="e.g. ~/private"
            @keydown.enter="addPath"
          />
          <AppButton :disabled="!newPath.trim()" @click="addPath">Add</AppButton>
        </div>
        <p v-if="pathError" class="error-text">{{ pathError }}</p>
        <div v-if="paths.length" class="chip-row">
          <span v-for="value in paths" :key="value" class="chip">
            {{ value }}
            <button class="chip-remove" :aria-label="`Remove ${value}`" @click="removePath(value)">
              &times;
            </button>
          </span>
        </div>
      </div>

      <div class="create-actions">
        <AppButton variant="primary" :disabled="!dirty || saving" @click="save">
          {{ saving ? 'Saving...' : 'Save policy' }}
        </AppButton>
        <span v-if="savedNotice && !dirty" class="test-status test-status-ok">
          {{ savedNotice }}
        </span>
      </div>
      <p v-if="saveError" class="error-text">{{ saveError }}</p>
    </template>
  </div>
</template>

<style scoped>
.policy-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.policy-rule-id {
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  color: var(--color-fg-muted);
}

.policy-mode {
  min-width: 10rem;
}
</style>

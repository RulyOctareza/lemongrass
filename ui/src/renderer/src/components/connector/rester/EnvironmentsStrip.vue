<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { VaultEnvironment, VaultRester } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import EnvironmentModal from './EnvironmentModal.vue'
import type { CommitRester } from './resterDraft'

const props = defineProps<{
  rester: VaultRester
  commit: CommitRester
}>()

type ModalMode =
  { kind: 'add' } | { kind: 'edit'; source: string } | { kind: 'duplicate'; source: string }

const mode = ref<ModalMode | null>(null)
const saving = ref(false)
const modalError = ref('')
const stripError = ref('')
const deleting = ref<string | null>(null)

const names = computed(() => props.rester.Environments.map((e) => e.Name))

const modalTitle = computed(() => {
  const m = mode.value
  if (!m || m.kind === 'add') return 'New environment'
  return m.kind === 'edit' ? `Edit ${m.source}` : `New environment from ${m.source}`
})

const modalInitial = computed<VaultEnvironment>(() => {
  const m = mode.value
  if (!m || m.kind === 'add') return { Name: '', BaseURL: '', Origin: '', Headers: null }
  const source = props.rester.Environments.find((e) => e.Name === m.source)
  const base = source ?? { Name: m.source, BaseURL: '', Origin: '', Headers: null }
  return { ...base, Name: m.kind === 'duplicate' ? `${m.source} copy` : base.Name }
})

function open(next: ModalMode): void {
  modalError.value = ''
  mode.value = next
}

function assignedCount(env: string): number {
  return props.rester.Users.filter((u) => (u.Environments ?? []).includes(env)).length
}

async function save(environment: VaultEnvironment): Promise<void> {
  const m = mode.value
  if (!m) return
  saving.value = true
  modalError.value = ''
  modalError.value = await props.commit((r) => {
    if (m.kind === 'edit') {
      const index = r.Environments.findIndex((e) => e.Name === m.source)
      if (index !== -1) r.Environments[index] = environment
      return
    }
    r.Environments.push(environment)
    if (m.kind !== 'duplicate') return
    // Duplicate also copies who is assigned to the source and their overrides there.
    for (const user of r.Users) {
      if (!(user.Environments ?? []).includes(m.source)) continue
      user.Environments = [...(user.Environments ?? []), environment.Name]
      const override = user.Overrides?.[m.source]
      if (override) user.Overrides = { ...user.Overrides, [environment.Name]: { ...override } }
    }
  })
  saving.value = false
  if (modalError.value === '') mode.value = null
}

async function remove(name: string): Promise<void> {
  deleting.value = name
  stripError.value = await props.commit((r) => {
    r.Environments = r.Environments.filter((e) => e.Name !== name)
    for (const user of r.Users) {
      user.Environments = (user.Environments ?? []).filter((e) => e !== name)
      if (user.Overrides && name in user.Overrides) {
        const rest = { ...user.Overrides }
        delete rest[name]
        user.Overrides = Object.keys(rest).length > 0 ? rest : null
      }
    }
  })
  deleting.value = null
}
</script>

<template>
  <div class="inline-section">
    <div class="users-toolbar">
      <span class="field-label">Environments ({{ rester.Environments.length }})</span>
      <span class="toolbar-fill"></span>
      <AppButton @click="open({ kind: 'add' })">+ Environment</AppButton>
    </div>

    <div class="env-grid">
      <div v-for="env in rester.Environments" :key="env.Name" class="env-card">
        <span class="channel-db">{{ env.Name }}</span>
        <span class="env-line" :class="{ missing: !env.BaseURL }">
          API: {{ env.BaseURL || 'no base URL' }}
        </span>
        <span class="env-line" :class="{ missing: !env.Origin }">
          Web: {{ env.Origin || 'no origin' }}
        </span>
        <span class="env-line">
          {{ assignedCount(env.Name) }} users, {{ (env.Headers ?? []).length }} extra headers
        </span>
        <div class="channel-actions">
          <AppButton @click="open({ kind: 'edit', source: env.Name })">Edit</AppButton>
          <AppButton @click="open({ kind: 'duplicate', source: env.Name })">Duplicate</AppButton>
          <AppButton
            variant="danger"
            :disabled="rester.Environments.length < 2 || deleting === env.Name"
            :title="rester.Environments.length < 2 ? 'A rester keeps at least one environment' : ''"
            @click="remove(env.Name)"
          >
            {{ deleting === env.Name ? 'Deleting...' : 'Delete' }}
          </AppButton>
        </div>
      </div>
    </div>

    <p v-if="stripError" class="error-text">{{ stripError }}</p>

    <EnvironmentModal
      v-if="mode"
      :key="modalTitle"
      :title="modalTitle"
      :initial="modalInitial"
      :lock-name="mode.kind === 'edit'"
      :taken="names"
      :saving="saving"
      :error="modalError"
      @save="save"
      @close="mode = null"
    />
  </div>
</template>

<style scoped>
.toolbar-fill {
  flex: 1;
}

.env-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: var(--space-3);
}

.env-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  min-width: 0;
  padding: var(--space-3);
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
}

.env-line {
  overflow: hidden;
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.env-line.missing {
  color: var(--color-fg-muted);
}

.env-card .channel-actions {
  justify-content: flex-end;
  margin-top: var(--space-2);
}
</style>

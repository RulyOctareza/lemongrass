<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { switchBranch } from '../git'
import type { ProjectRef } from '../workspace'
import type { GitBranches } from '../../../preload/types'
import AppModal from './AppModal.vue'

// With pickOnly the modal names a branch for the caller instead of checking it out.
const props = defineProps<{
  project: ProjectRef
  pickOnly?: boolean
  title?: string
}>()

const emit = defineEmits<{
  close: []
  pick: [name: string]
}>()

const branches = ref<GitBranches | null>(null)
const filter = ref('')
const error = ref<string | null>(null)
const switching = ref(false)

onMounted(async () => {
  branches.value = await window.api.git.branches(props.project.path)
})

function matches(name: string): boolean {
  return name.toLowerCase().includes(filter.value.trim().toLowerCase())
}

const local = computed(() => (branches.value?.local ?? []).filter(matches))

// A remote branch that already has a local twin is reached through the local one.
const remote = computed(() => {
  const owned = new Set(props.pickOnly ? [] : (branches.value?.local ?? []))
  return (branches.value?.remote ?? []).filter(
    (name) => !owned.has(name.slice(name.indexOf('/') + 1)) && matches(name)
  )
})

async function pick(name: string, isRemote: boolean): Promise<void> {
  if (switching.value) return
  if (props.pickOnly) {
    emit('pick', name)
    emit('close')
    return
  }
  error.value = null
  switching.value = true
  error.value = await switchBranch(props.project, name, isRemote)
  switching.value = false
  if (!error.value) emit('close')
}
</script>

<template>
  <AppModal :title="title ?? 'Switch branch'" @close="emit('close')">
    <input
      v-model="filter"
      class="branch-filter"
      placeholder="Filter branches"
      aria-label="Filter branches"
      autofocus
    />
    <p v-if="error" class="branch-error">{{ error }}</p>
    <p v-if="!branches" class="branch-empty">Loading...</p>
    <div v-else class="branch-lists">
      <template v-if="local.length > 0">
        <p class="branch-heading">Local</p>
        <button
          v-for="name in local"
          :key="`local-${name}`"
          class="branch-row"
          :class="{ current: name === branches.current }"
          :disabled="name === branches.current || switching"
          @click="pick(name, false)"
        >
          {{ name }}
          <span v-if="name === branches.current" class="branch-tag">current</span>
        </button>
      </template>
      <template v-if="remote.length > 0">
        <p class="branch-heading">Remote</p>
        <button
          v-for="name in remote"
          :key="`remote-${name}`"
          class="branch-row"
          :disabled="switching"
          title="Creates a local branch that tracks it"
          @click="pick(name, true)"
        >
          {{ name }}
        </button>
      </template>
      <p v-if="local.length === 0 && remote.length === 0" class="branch-empty">
        No branches match.
      </p>
    </div>
  </AppModal>
</template>

<style scoped>
.branch-filter {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
}

.branch-filter:focus {
  outline: none;
  border-color: var(--color-amber);
}

.branch-lists {
  display: flex;
  flex-direction: column;
  max-height: 50vh;
  margin-top: var(--space-3);
  overflow-y: auto;
}

.branch-heading {
  padding: var(--space-2) var(--space-2) var(--space-1);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.branch-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 4px var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.branch-row:hover:not(:disabled) {
  background: var(--color-surface-2);
}

.branch-row:disabled {
  cursor: default;
}

.branch-row.current {
  color: var(--color-fg-accent);
}

.branch-tag {
  margin-left: auto;
  color: var(--color-fg-muted);
}

.branch-empty {
  margin-top: var(--space-3);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.branch-error {
  margin-top: var(--space-3);
  max-height: 120px;
  overflow-y: auto;
  padding: var(--space-2);
  background: var(--color-error-muted);
  border-radius: var(--radius-md);
  color: var(--color-error);
  font-size: var(--text-xs);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { userHandle } from '../../../userHandle'
import type { VaultRester, VaultResterUser } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import AppCheckbox from '../../AppCheckbox.vue'
import UserModal from './UserModal.vue'
import type { CommitRester } from './resterDraft'

const props = defineProps<{
  rester: VaultRester
  commit: CommitRester
}>()

const search = ref('')
const modalUser = ref<VaultResterUser | null>(null)
const modalOpen = ref(false)
const busyUser = ref<string | null>(null)
const listError = ref('')

const visibleUsers = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (query === '') return props.rester.Users
  return props.rester.Users.filter(
    (u) =>
      u.Name.toLowerCase().includes(query) ||
      userHandle(u.Name).includes(query) ||
      (u.Tags ?? []).some((t) => t.toLowerCase().includes(query))
  )
})

function openUser(user: VaultResterUser | null): void {
  modalUser.value = user
  modalOpen.value = true
}

function assigned(user: VaultResterUser, env: string): boolean {
  return (user.Environments ?? []).includes(env)
}

// Unassigning also drops that environment's override, since the vault keeps overrides only for assigned environments.
async function toggleAssignment(user: VaultResterUser, env: string): Promise<void> {
  if (busyUser.value) return
  busyUser.value = user.Name
  const add = !assigned(user, env)
  const order = props.rester.Environments.map((e) => e.Name)
  listError.value = await props.commit((r) => {
    const target = r.Users.find((u) => u.Name === user.Name)
    if (!target) return
    const current = target.Environments ?? []
    const next = add ? [...current, env] : current.filter((e) => e !== env)
    target.Environments = order.filter((e) => next.includes(e))
    if (!add && target.Overrides && env in target.Overrides) {
      const rest = { ...target.Overrides }
      delete rest[env]
      target.Overrides = Object.keys(rest).length > 0 ? rest : null
    }
  })
  busyUser.value = null
}
</script>

<template>
  <div class="inline-section">
    <div class="users-toolbar">
      <span class="field-label">Users ({{ rester.Users.length }})</span>
      <input
        v-model="search"
        type="search"
        class="users-search"
        placeholder="Search name, handle or tag"
      />
      <AppButton @click="openUser(null)">+ User</AppButton>
    </div>

    <p v-if="listError" class="error-text">{{ listError }}</p>
    <p v-if="rester.Users.length === 0" class="empty">No users yet.</p>
    <p v-else-if="visibleUsers.length === 0" class="empty">No user matches.</p>

    <div v-for="user in visibleUsers" :key="user.Name" class="user-draft user-row">
      <button type="button" class="user-summary" @click="openUser(user)">
        <span class="user-summary-name">{{ user.Name }}</span>
        <span class="capsule-row">
          <span v-for="tag in user.Tags ?? []" :key="tag" class="value-capsule">{{ tag }}</span>
        </span>
        <span class="user-summary-kind">{{ user.Token !== '' ? 'Token' : 'Login' }}</span>
      </button>
      <div class="env-checks">
        <span v-if="(user.Environments ?? []).length === 0" class="status-capsule status-expired">
          Unassigned
        </span>
        <AppCheckbox
          v-for="env in rester.Environments"
          :key="env.Name"
          :checked="assigned(user, env.Name)"
          :disabled="busyUser !== null"
          @click="toggleAssignment(user, env.Name)"
        >
          {{ env.Name }}
        </AppCheckbox>
      </div>
    </div>

    <UserModal
      v-if="modalOpen"
      :key="modalUser?.Name ?? ''"
      :rester="rester"
      :user="modalUser"
      :commit="commit"
      @close="modalOpen = false"
    />
  </div>
</template>

<style scoped>
.user-row {
  flex-direction: row;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 0;
}

.user-row .user-summary {
  flex: 1;
  width: auto;
}

.env-checks {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
}
</style>

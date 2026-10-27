<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import AppModal from './AppModal.vue'
import { closeJoin, confirmJoin, joining } from '../workgroups'
import AppButton from './AppButton.vue'

const cancelButton = ref<InstanceType<typeof AppButton> | null>(null)

watch(
  () => joining.draft,
  (draft) => {
    if (draft) void nextTick(() => cancelButton.value?.focus())
  }
)
</script>

<template>
  <AppModal
    v-if="joining.draft"
    scope="app"
    size="md"
    title="Add tab to workgroup"
    @close="closeJoin"
  >
    <dl class="facts">
      <div class="fact">
        <dt>Tab</dt>
        <dd>{{ joining.draft.tabTitle }}</dd>
      </div>
      <div class="fact">
        <dt>Workgroup</dt>
        <dd>{{ joining.draft.groupName }}</dd>
      </div>
      <div class="fact">
        <dt>Joins as</dt>
        <dd><span class="role-pill">Collaborator</span></dd>
      </div>
    </dl>
    <p class="explain">
      The session keeps its context, and you keep talking to it in its own tab. It joins the group
      thread as a collaborator. It shares what it knows and coordinates with the group, but takes no
      assignment from the leader.
    </p>
    <p v-if="joining.notice" class="notice">{{ joining.notice }}</p>
    <p v-else-if="joining.error" class="error">{{ joining.error }}</p>
    <template #footer>
      <template v-if="joining.notice">
        <AppButton ref="cancelButton" variant="primary" @click="closeJoin">Close</AppButton>
      </template>
      <template v-else>
        <AppButton ref="cancelButton" :disabled="joining.busy" @click="closeJoin">
          Cancel
        </AppButton>
        <AppButton variant="primary" :disabled="joining.busy" @click="confirmJoin">
          {{ joining.busy ? 'Adding...' : 'Approve' }}
        </AppButton>
      </template>
    </template>
  </AppModal>
</template>

<style scoped>
.facts {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: var(--space-2) var(--space-4);
  margin: 0;
}

.fact {
  display: contents;
}

.fact dt {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  line-height: 20px;
}

.fact dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  line-height: 20px;
}

.role-pill {
  padding: 0 var(--space-2);
  background: var(--color-sapphire-muted);
  border-radius: var(--radius-pill);
  color: var(--color-sapphire);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  line-height: 18px;
}

.explain {
  margin: var(--space-4) 0 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.notice,
.error {
  margin: var(--space-3) 0 0;
  font-size: var(--text-sm);
}

.notice {
  color: var(--color-fg-secondary);
}

.error {
  color: var(--color-error);
}
</style>

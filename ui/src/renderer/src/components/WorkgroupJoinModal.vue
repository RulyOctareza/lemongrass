<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import AppModal from './AppModal.vue'
import { closeJoin, confirmJoin, joining } from '../workgroups'

const cancelButton = ref<HTMLButtonElement | null>(null)

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
        <dt>Label</dt>
        <dd>{{ joining.draft.label }}</dd>
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
        <button ref="cancelButton" class="primary-button" @click="closeJoin">Close</button>
      </template>
      <template v-else>
        <button ref="cancelButton" class="ghost-button" :disabled="joining.busy" @click="closeJoin">
          Cancel
        </button>
        <button class="primary-button" :disabled="joining.busy" @click="confirmJoin">
          {{ joining.busy ? 'Adding...' : 'Approve' }}
        </button>
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

.ghost-button,
.primary-button {
  padding: var(--space-1) var(--space-4);
  border-radius: var(--radius-pill);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.ghost-button {
  background: transparent;
  border: 1px solid var(--color-border-default);
  color: var(--color-fg-secondary);
}

.ghost-button:hover:not(:disabled) {
  color: var(--color-fg-primary);
  background: var(--color-surface-2);
}

.ghost-button:focus-visible,
.primary-button:focus-visible {
  outline: 2px solid var(--color-fg-accent);
  outline-offset: 2px;
}

.primary-button {
  background: var(--color-amber);
  border: none;
  color: var(--color-black);
}

.primary-button:hover:not(:disabled) {
  background: var(--color-amber-dim);
}

.ghost-button:disabled,
.primary-button:disabled {
  opacity: 0.5;
  cursor: default;
}
</style>

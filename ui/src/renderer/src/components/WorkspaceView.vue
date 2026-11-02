<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import AppModal from './AppModal.vue'
import WorkspaceLayout from './WorkspaceLayout.vue'
import {
  addShell,
  cancelShellPicker,
  confirmShellPicker,
  shellPicker,
  workspaceOf,
  type ProjectRef
} from '../workspace'
import { accountStore } from '../accounts'
import { SHELL_AGENTS, shellAgentById } from '../shellAgents'
import AppButton from './AppButton.vue'
import AppEmptyState from './AppEmptyState.vue'

const props = defineProps<{
  project: ProjectRef
}>()

const workspace = computed(() => workspaceOf(props.project.id))
const selectedAgentWarning = computed(() => shellAgentById(shellPicker.selectedId).warning)
const showAccounts = computed(
  () => shellPicker.selectedId === 'claude' && accountStore.items.length > 0
)
</script>

<template>
  <div class="workspace-view">
    <template v-if="workspace?.layout.root">
      <div class="layout-area">
        <WorkspaceLayout
          :node="workspace.layout.root"
          :project="project"
          :focused-pane-id="workspace.layout.focusedPaneId"
        />
      </div>
    </template>
    <AppEmptyState v-else layout="fill" title="Nothing open.">
      <template #action>
        <AppButton variant="primary" @click="addShell(project)">+ New shell</AppButton>
      </template>
    </AppEmptyState>

    <AppModal v-if="shellPicker.open" title="New shell" @close="cancelShellPicker">
      <p class="card-text">Which agent should this pane run?</p>
      <div class="agent-list" role="listbox" aria-label="Shell agent">
        <button
          v-for="agent in SHELL_AGENTS"
          :key="agent.id"
          type="button"
          class="agent-option"
          :class="{ selected: shellPicker.selectedId === agent.id }"
          role="option"
          :aria-selected="shellPicker.selectedId === agent.id"
          @click="shellPicker.selectedId = agent.id"
          @dblclick="confirmShellPicker(agent.id)"
        >
          <span class="agent-label">{{ agent.label }}</span>
          <span class="agent-hint">{{ agent.hint }}</span>
        </button>
      </div>
      <template v-if="showAccounts">
        <p class="card-text">Which account should it use?</p>
        <div class="agent-list" role="listbox" aria-label="Claude account">
          <button
            type="button"
            class="agent-option"
            :class="{ selected: shellPicker.account === undefined }"
            role="option"
            :aria-selected="shellPicker.account === undefined"
            @click="shellPicker.account = undefined"
            @dblclick="confirmShellPicker()"
          >
            <span class="agent-label">primary</span>
            <span class="agent-hint">~/.claude</span>
          </button>
          <button
            v-for="account in accountStore.items"
            :key="account.name"
            type="button"
            class="agent-option"
            :class="{ selected: shellPicker.account === account.name }"
            role="option"
            :aria-selected="shellPicker.account === account.name"
            @click="shellPicker.account = account.name"
            @dblclick="confirmShellPicker()"
          >
            <span class="agent-label">{{ account.name }}</span>
            <span class="agent-hint">{{ account.dir }}</span>
          </button>
        </div>
      </template>
      <p v-if="selectedAgentWarning" class="agent-warning">{{ selectedAgentWarning }}</p>
      <template #footer>
        <AppButton @click="cancelShellPicker">Cancel</AppButton>
        <AppButton variant="primary" @click="confirmShellPicker()">Open</AppButton>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.workspace-view {
  position: relative;
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.layout-area {
  flex: 1;
  min-height: 0;
  display: flex;
}

.card-text {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.agent-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.agent-option {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-3);
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 8px);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  cursor: pointer;
  text-align: left;
  transition:
    background var(--duration-fast) var(--ease-out),
    border-color var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.agent-option:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.agent-option.selected {
  border-color: var(--color-amber);
  background: var(--color-amber-muted, rgba(245, 197, 24, 0.12));
  color: var(--color-fg-primary);
}

.agent-label {
  font-weight: var(--weight-medium);
}

.agent-hint {
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  color: var(--color-fg-muted);
}

.agent-warning {
  margin: 0;
  color: var(--color-amber);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}
</style>

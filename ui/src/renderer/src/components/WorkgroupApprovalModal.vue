<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import AppModal from './AppModal.vue'
import { approvals, closeApprovals, decideApproval, ensureAccountChoices } from '../workgroups'
import { accountStore } from '../accounts'
import { shellAccountOf } from '../workspace'
import { renderSafeMarkdown } from '../safeMarkdown'
import type { WorkgroupPending } from '../../../preload/types'
import AppButton from './AppButton.vue'

const declineButton = ref<InstanceType<typeof AppButton> | null>(null)
const reasonInput = ref<HTMLTextAreaElement | null>(null)

const selected = computed((): WorkgroupPending | undefined =>
  approvals.pending.find((request) => request.requestId === approvals.selectedId)
)

function projectName(request: WorkgroupPending): string {
  const parts = request.projectPath.split(/[\\/]/).filter(Boolean)
  return parts[parts.length - 1] ?? request.projectPath
}

function thinkerCount(request: WorkgroupPending): string {
  const count = request.members.length
  return `${count} thinker${count === 1 ? '' : 's'}`
}

// Every request names its kind, so an addition to a live group is never read as a new one.
function kindLabel(request: WorkgroupPending): string {
  return request.existingMembers ? 'Adding thinkers' : 'New workgroup'
}

function focusDecline(): void {
  void nextTick(() => declineButton.value?.focus())
}

function onDecline(requestId: string): void {
  if (!approvals.declining[requestId]) {
    approvals.declining[requestId] = true
    void nextTick(() => reasonInput.value?.focus())
    return
  }
  void decideApproval(requestId, false)
}

watch(
  selected,
  (request) => {
    if (request)
      ensureAccountChoices(request, shellAccountOf(request.projectPath, request.leaderTabId))
  },
  { immediate: true }
)

watch(
  () => [approvals.open, approvals.selectedId],
  () => {
    if (approvals.open) focusDecline()
  }
)
</script>

<template>
  <AppModal
    v-if="approvals.open"
    scope="app"
    size="lg"
    flush
    label="Workgroups waiting for approval"
    @close="closeApprovals"
  >
    <div class="approval-layout">
      <aside class="request-list">
        <div class="list-head">
          <h3 class="list-title">Waiting approval</h3>
        </div>
        <ul class="request-items lg-scroll">
          <li v-for="request in approvals.pending" :key="request.requestId">
            <button
              class="request-row"
              :class="{ selected: request.requestId === approvals.selectedId }"
              @click="approvals.selectedId = request.requestId"
            >
              <span class="request-title">
                <span class="request-name">{{ request.groupName }}</span>
                <span class="kind-pill" :class="{ addition: request.existingMembers }">
                  {{ kindLabel(request) }}
                </span>
              </span>
              <span class="request-meta">
                {{ projectName(request) }}, {{ thinkerCount(request) }}
              </span>
            </button>
          </li>
        </ul>
      </aside>

      <section v-if="selected" class="reading">
        <header class="reading-head">
          <div class="reading-title-row">
            <h2 class="reading-title">{{ selected.groupName }}</h2>
            <span class="kind-pill" :class="{ addition: selected.existingMembers }">
              {{ kindLabel(selected) }}
            </span>
          </div>
          <dl class="facts">
            <div class="fact">
              <dt>Project</dt>
              <dd>{{ projectName(selected) }}</dd>
            </div>
            <div class="fact">
              <dt>{{ selected.existingMembers ? 'Leader' : 'Started by' }}</dt>
              <dd>{{ selected.leaderLabel }}</dd>
            </div>
            <div class="fact">
              <dt>{{ selected.existingMembers ? 'New thinkers' : 'Thinkers' }}</dt>
              <dd>{{ selected.members.length }}</dd>
            </div>
            <div v-if="selected.existingMembers" class="fact">
              <dt>Already in the group</dt>
              <dd>{{ selected.existingMembers.join(', ') }}</dd>
            </div>
          </dl>
        </header>

        <div class="reading-body lg-scroll">
          <details
            v-for="(member, index) in selected.members"
            :key="`${selected.requestId}-${member.label}`"
            class="member"
            :open="index === 0"
          >
            <summary class="member-head">
              <span class="member-label">{{ member.label }}</span>
              <span class="vendor-badge">{{ member.vendor }}</span>
              <span v-if="member.model" class="model-name">{{ member.model }}</span>
            </summary>
            <div v-if="member.skills.length" class="skills">
              <span class="skills-label">Must load</span>
              <span v-for="skill in member.skills" :key="skill" class="chip">{{ skill }}</span>
            </div>
            <div
              v-if="
                member.vendor === 'claude' &&
                accountStore.items.length > 0 &&
                approvals.accounts[selected.requestId]
              "
              class="account"
            >
              <span :id="`account-${selected.requestId}-${index}`" class="account-label">
                Account
              </span>
              <div
                class="segmented"
                role="radiogroup"
                :aria-labelledby="`account-${selected.requestId}-${index}`"
              >
                <button
                  v-for="name in ['', ...accountStore.items.map((account) => account.name)]"
                  :key="name || 'primary'"
                  type="button"
                  role="radio"
                  :aria-checked="approvals.accounts[selected.requestId][index] === name"
                  @click="approvals.accounts[selected.requestId][index] = name"
                >
                  {{ name || 'primary' }}
                </button>
              </div>
            </div>
            <p class="assignment-label">Assignment</p>
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div class="assignment" v-html="renderSafeMarkdown(member.prompt)" />
          </details>
        </div>

        <div v-if="approvals.declining[selected.requestId]" class="reason">
          <label class="reason-label" :for="`reason-${selected.requestId}`">
            Reason for declining, optional. Press Decline again to send it.
          </label>
          <textarea
            :id="`reason-${selected.requestId}`"
            ref="reasonInput"
            v-model="approvals.reasons[selected.requestId]"
            class="reason-input"
            rows="2"
            maxlength="500"
            placeholder="The agent that proposed this group will read it."
          />
        </div>

        <footer class="action-bar">
          <AppButton
            ref="declineButton"
            :disabled="approvals.busy"
            @click="onDecline(selected.requestId)"
          >
            Decline
          </AppButton>
          <AppButton
            variant="primary"
            :disabled="approvals.busy"
            @click="decideApproval(selected.requestId, true)"
          >
            Approve
          </AppButton>
        </footer>
      </section>
    </div>
  </AppModal>
</template>

<style scoped>
.approval-layout {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  flex: 1;
  min-height: 0;
}

.request-list {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--color-surface-0);
  border-right: 1px solid var(--color-border-default);
}

.list-head {
  display: flex;
  align-items: center;
  padding: var(--space-4) var(--space-4) var(--space-3);
}

.list-title {
  color: var(--color-fg-primary);
  font-family: var(--font-display);
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
}

.request-items {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 0 var(--space-2) var(--space-3);
}

.request-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  text-align: left;
  cursor: pointer;
}

.request-row:hover {
  background: var(--color-surface-2);
}

.request-row.selected {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.request-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}

.request-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
}

.request-meta {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.reading {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

.reading-head {
  padding: var(--space-5) var(--space-10) var(--space-4) var(--space-6);
  border-bottom: 1px solid var(--color-border-subtle);
}

.reading-title-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}

.kind-pill {
  flex-shrink: 0;
  padding: 0 var(--space-2);
  background: var(--color-surface-3);
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  line-height: 18px;
  white-space: nowrap;
}

.kind-pill.addition {
  background: var(--color-sapphire-muted);
  color: var(--color-sapphire);
}

.reading-title {
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--color-fg-primary);
  font-family: var(--font-display);
  font-size: var(--text-lg);
  font-weight: var(--weight-bold);
}

.facts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-5);
  margin: var(--space-3) 0 0;
}

.fact dt {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.fact dd {
  margin: 0;
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
}

.reading-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4) var(--space-6);
}

.member {
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-lg);
  padding: var(--space-3) var(--space-4);
}

.member-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
}

.member-label {
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
}

.vendor-badge {
  padding: 0 var(--space-2);
  background: var(--color-amber-muted);
  border-radius: var(--radius-pill);
  color: var(--color-fg-accent);
  font-size: var(--text-xs);
  line-height: 18px;
}

.model-name {
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.skills {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  margin-top: var(--space-3);
}

.account {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-top: var(--space-3);
}

.segmented {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border-radius: var(--radius-md);
  background: var(--color-surface-3);
}

.segmented button {
  padding: var(--space-1) var(--space-3);
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-default),
    color var(--duration-fast) var(--ease-default);
}

.segmented button:hover {
  color: var(--color-fg-primary);
}

.segmented button[aria-checked='true'] {
  background: var(--color-surface-1);
  color: var(--color-fg-primary);
  font-weight: var(--weight-semibold);
  box-shadow: var(--shadow-sm);
}

.segmented button:focus-visible {
  outline: 1px solid var(--color-amber);
  outline-offset: 1px;
}

.skills-label,
.account-label,
.assignment-label {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.assignment-label {
  margin: var(--space-3) 0 var(--space-1);
}

.chip {
  padding: 0 var(--space-2);
  background: var(--color-surface-3);
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  line-height: 18px;
}

.assignment {
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
  overflow-wrap: anywhere;
}

.assignment :deep(h1),
.assignment :deep(h2),
.assignment :deep(h3),
.assignment :deep(h4) {
  margin: var(--space-4) 0 var(--space-2);
  font-family: var(--font-display);
  font-size: var(--text-md);
  font-weight: var(--weight-bold);
  line-height: var(--leading-tight);
}

.assignment :deep(> :first-child) {
  margin-top: 0;
}

.assignment :deep(p),
.assignment :deep(ul),
.assignment :deep(ol),
.assignment :deep(pre),
.assignment :deep(blockquote),
.assignment :deep(table) {
  margin: 0 0 var(--space-3);
}

.assignment :deep(ul),
.assignment :deep(ol) {
  padding-left: var(--space-5);
}

.assignment :deep(li) {
  margin-bottom: var(--space-1);
}

.assignment :deep(strong) {
  font-weight: var(--weight-semibold);
}

.assignment :deep(code) {
  padding: 0.15em 0.4em;
  background: var(--color-surface-3);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 0.9em;
}

.assignment :deep(pre) {
  padding: var(--space-3);
  overflow-x: auto;
  background: var(--color-surface-0);
  border-radius: var(--radius-md);
}

.assignment :deep(pre code) {
  padding: 0;
  background: none;
}

.assignment :deep(blockquote) {
  padding-left: var(--space-4);
  border-left: 2px solid var(--color-border-default);
  color: var(--color-fg-secondary);
}

.assignment :deep(.md-address) {
  color: var(--color-fg-muted);
  font-family: var(--font-mono);
  font-size: 0.9em;
}

.assignment :deep(.md-image) {
  color: var(--color-fg-secondary);
  font-style: italic;
}

.assignment :deep(table) {
  border-collapse: collapse;
}

.assignment :deep(th),
.assignment :deep(td) {
  padding: var(--space-1) var(--space-3);
  border: 1px solid var(--color-border-default);
  text-align: left;
}

.reason {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-6) 0;
  border-top: 1px solid var(--color-border-subtle);
}

.reason-label {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.reason-input {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  resize: none;
}

.reason-input:focus {
  outline: none;
  border-color: var(--color-amber);
}

.action-bar {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-6);
}
</style>

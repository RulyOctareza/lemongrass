<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppModal from './AppModal.vue'
import { bodyParts, type BodyPart } from '../mentions'
import { disbandWorkgroup, refreshWorkgroups, workgroupStateOf } from '../workgroups'
import { GROUP_COLORS, groupColorVar } from '../groupColors'
import {
  focusTab,
  groupColorOf,
  isTabOpen,
  reopenWorkgroupMember,
  setGroupColor,
  type ProjectRef
} from '../workspace'
import type {
  WorkgroupInfo,
  WorkgroupMemberInfo,
  WorkgroupThreadMessage
} from '../../../preload/types'
import AppButton from './AppButton.vue'

const POLL_MS = 4000
const PAGE = 50

const props = defineProps<{
  project: ProjectRef
  groupId: number
  active: boolean
}>()

const state = computed(() => workgroupStateOf(props.project.id))
const live = computed(() => state.value.groups.find((g) => g.id === props.groupId))
const remembered = ref<WorkgroupInfo | null>(null)
const loaded = ref(false)
const group = computed(() => live.value ?? remembered.value)

const confirming = ref(false)
const messages = ref<WorkgroupThreadMessage[]>([])
const more = ref(false)
const threadError = ref<string | null>(null)
const loadingOlder = ref(false)
const scroller = ref<HTMLDivElement>()
const cancelButton = ref<InstanceType<typeof AppButton> | null>(null)

const color = computed(() => groupColorOf(props.project.id, props.groupId))

const ROLE_LABELS: Record<WorkgroupMemberInfo['role'], string> = {
  leader: 'Leader',
  thinker: 'Thinker',
  collaborator: 'Collaborator'
}

watch(
  live,
  (value) => {
    if (value) remembered.value = value
  },
  { immediate: true }
)

function online(member: WorkgroupMemberInfo): boolean {
  return isTabOpen(props.project.id, member.tabId)
}

function onMember(member: WorkgroupMemberInfo): void {
  if (online(member)) focusTab(props.project, member.tabId)
  else if (group.value) void reopenWorkgroupMember(props.project, member, group.value.leaderTabId)
}

function messageParts(body: string): BodyPart[] {
  return bodyParts(body, group.value?.members ?? [])
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function nearBottom(): boolean {
  const el = scroller.value
  return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

function merge(incoming: WorkgroupThreadMessage[]): void {
  const byId = new Map(messages.value.map((m) => [m.id, m]))
  for (const m of incoming) byId.set(m.id, m)
  messages.value = [...byId.values()].sort((a, b) => a.id - b.id)
}

async function refreshThread(): Promise<void> {
  const threadId = group.value?.threadId
  if (!threadId) return
  const stick = nearBottom()
  const result = await window.api.workgroup.thread(props.project.path, threadId, undefined, PAGE)
  if (!result.ok || !result.thread) {
    threadError.value = result.error ?? 'Could not read the thread.'
    return
  }
  threadError.value = null
  const first = messages.value.length === 0
  merge(result.thread.messages)
  if (first) more.value = result.thread.more
  if (first || stick) {
    await nextTick()
    if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight
  }
}

async function loadOlder(): Promise<void> {
  const threadId = group.value?.threadId
  const oldest = messages.value[0]
  if (!threadId || !oldest || loadingOlder.value) return
  loadingOlder.value = true
  try {
    const el = scroller.value
    const previousHeight = el?.scrollHeight ?? 0
    const result = await window.api.workgroup.thread(props.project.path, threadId, oldest.id, PAGE)
    if (!result.ok || !result.thread) {
      threadError.value = result.error ?? 'Could not read the thread.'
      return
    }
    merge(result.thread.messages)
    more.value = result.thread.more
    await nextTick()
    if (el) el.scrollTop = el.scrollHeight - previousHeight
  } finally {
    loadingOlder.value = false
  }
}

async function refresh(): Promise<void> {
  await refreshWorkgroups(props.project)
  loaded.value = true
  await refreshThread()
}

async function onDisband(): Promise<void> {
  await disbandWorkgroup(props.project, props.groupId)
  confirming.value = false
}

watch(confirming, async (open) => {
  if (!open) return
  await nextTick()
  cancelButton.value?.focus()
})

let timer: ReturnType<typeof setInterval> | undefined

function start(): void {
  void refresh()
  clearInterval(timer)
  timer = setInterval(() => {
    if (document.hasFocus()) void refresh()
  }, POLL_MS)
}

watch(
  () => props.active,
  (active) => {
    if (active) start()
    else clearInterval(timer)
  }
)

onMounted(() => {
  if (props.active) start()
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div class="workgroup-pane">
    <p v-if="!group && !loaded" class="pane-note">Loading...</p>
    <p v-else-if="!group" class="pane-note">
      This workgroup is no longer live. Close this tab to remove it.
    </p>

    <template v-else>
      <header class="pane-head">
        <div class="title-block">
          <h2 class="pane-title">{{ group.name }}</h2>
          <p class="pane-sub">
            <span class="state" :class="{ live: !!live }">{{ live ? 'Live' : 'Disbanded' }}</span>
            <span v-if="live && !isTabOpen(project.id, group.leaderTabId)" class="orphaned">
              Orphaned, the leader's tab is not open
            </span>
          </p>
          <div class="swatches" role="radiogroup" aria-label="Group color">
            <button
              v-for="name in GROUP_COLORS"
              :key="name"
              class="swatch"
              :class="{ selected: name === color }"
              :style="{ background: groupColorVar(name) }"
              role="radio"
              :aria-checked="name === color"
              :title="name"
              @click="setGroupColor(project.id, groupId, name)"
            />
          </div>
        </div>
        <AppButton v-if="live" variant="danger" @click="confirming = true">Disband</AppButton>
      </header>

      <p v-if="state.error" class="pane-error">{{ state.error }}</p>

      <ul class="members">
        <li v-for="member in group.members" :key="member.tabId">
          <button
            class="member-row"
            :disabled="!live"
            :title="online(member) ? 'Focus this tab' : 'Reopen this tab and resume its session'"
            @click="onMember(member)"
          >
            <span class="member-role" :class="member.role">{{ ROLE_LABELS[member.role] }}</span>
            <span class="member-name">{{ member.label }}</span>
            <span class="member-vendor">{{ member.vendor }}</span>
            <span class="member-state" :class="{ online: online(member) }">
              {{ online(member) ? 'Online' : 'Offline' }}
            </span>
          </button>
        </li>
      </ul>

      <div class="thread">
        <h3 class="thread-title">Thread</h3>
        <div ref="scroller" class="thread-scroll lg-scroll">
          <button v-if="more" class="older-button" :disabled="loadingOlder" @click="loadOlder">
            {{ loadingOlder ? 'Loading...' : 'Load earlier messages' }}
          </button>
          <p v-if="threadError" class="pane-error">{{ threadError }}</p>
          <p v-else-if="messages.length === 0" class="pane-note">No messages yet.</p>
          <article v-for="message in messages" :key="message.id" class="message">
            <header class="message-head">
              <span class="message-label">{{ message.label }}</span>
              <time class="message-time">{{ formatTime(message.createdAt) }}</time>
            </header>
            <p class="message-body">
              <span
                v-for="(part, index) in messageParts(message.body)"
                :key="index"
                :class="{ mention: part.mention }"
                >{{ part.text }}</span
              >
            </p>
          </article>
        </div>
      </div>
    </template>

    <AppModal
      v-if="confirming && group"
      :title="`Disband ${group.name}?`"
      @close="confirming = false"
    >
      <p>Its tabs stay open and its thread stays readable.</p>
      <template #footer>
        <AppButton ref="cancelButton" :disabled="state.busy" @click="confirming = false">
          Cancel
        </AppButton>
        <AppButton variant="danger" :disabled="state.busy" @click="onDisband">Disband</AppButton>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.workgroup-pane {
  position: relative;
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-5) var(--space-6);
  overflow: hidden;
}

.pane-note {
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}

.pane-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
}

.title-block {
  min-width: 0;
}

.pane-title {
  overflow-wrap: anywhere;
  color: var(--color-fg-primary);
  font-family: var(--font-display);
  font-size: var(--text-lg);
  font-weight: var(--weight-bold);
}

.pane-sub {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  margin-top: var(--space-1);
  font-size: var(--text-xs);
}

.swatches {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-3);
}

.swatch {
  width: 16px;
  height: 16px;
  padding: 0;
  border: 2px solid transparent;
  border-radius: var(--radius-pill);
  background-clip: padding-box;
  outline: 1px solid transparent;
  outline-offset: 2px;
  cursor: pointer;
  transition: outline-color var(--duration-fast) var(--ease-out);
}

.swatch:hover {
  outline-color: var(--color-border-strong);
}

.swatch.selected {
  outline-color: var(--color-fg-primary);
}

.state {
  color: var(--color-fg-muted);
}

.state.live {
  color: var(--color-success);
}

.orphaned {
  color: var(--color-warning);
}

.pane-error {
  padding: var(--space-2) var(--space-3);
  background: var(--color-error-muted);
  border-radius: var(--radius-md);
  color: var(--color-error);
  font-size: var(--text-xs);
  white-space: pre-wrap;
  word-break: break-word;
}

.members {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.member-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  padding: var(--space-1) var(--space-3);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
}

.member-row:hover:not(:disabled) {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.member-row:disabled {
  color: var(--color-fg-muted);
  cursor: default;
}

.member-role {
  flex-shrink: 0;
  width: 70px;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.member-role.leader {
  color: var(--color-fg-accent);
}

.member-name {
  flex-shrink: 0;
  color: var(--color-fg-primary);
  font-weight: var(--weight-medium);
}

.member-vendor {
  flex: 1;
  min-width: 0;
  color: var(--color-fg-muted);
}

.member-state.online {
  color: var(--color-success);
}

.thread {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  border-top: 1px solid var(--color-border-default);
  padding-top: var(--space-3);
}

.thread-title {
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.thread-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding-right: var(--space-2);
}

.older-button {
  align-self: center;
  padding: var(--space-1) var(--space-3);
  background: transparent;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  cursor: pointer;
}

.older-button:hover:not(:disabled) {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.message {
  padding: var(--space-3) var(--space-4);
  background: var(--color-surface-2);
  border-radius: var(--radius-lg);
}

.message-head {
  display: flex;
  align-items: baseline;
  gap: var(--space-3);
  margin-bottom: var(--space-1);
}

.message-label {
  color: var(--color-fg-accent);
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
}

.message-time {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.message-body .mention {
  color: var(--color-fg-accent);
  font-weight: var(--weight-medium);
}

.message-body {
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>

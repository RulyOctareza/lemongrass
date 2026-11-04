<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import AppIcon from '../AppIcon.vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppButton from '../AppButton.vue'
import AppSwitcher from '../AppSwitcher.vue'
import AppTextarea from '../AppTextarea.vue'
import { clearReview, reviewOf, setReviewMessage } from '../../annotations'
import { handConflictTo, postToConflict, type ConflictThreadState } from '../../conflictThread'
import { bodyParts, type BodyPart } from '../../mentions'
import { resolveTargetsOf } from '../../reviewTargets'
import { shellTitles } from '../../shellRegistry'
import type { ProjectRef } from '../../workspace'
import type {
  ConflictHumanMessage,
  TabBusy,
  WorkgroupThreadMessage
} from '../../../../preload/types'
import AppEmptyState from '../AppEmptyState.vue'

const props = defineProps<{
  project: ProjectRef
  scope: string
  thread: ConflictThreadState
}>()

// Each poll runs lgrassd once, so this is as often as the picker can afford.
const BUSY_POLL_MS = 2500

const review = computed(() => reviewOf(props.project.id, props.scope))
const count = computed(() => review.value.annotations.length)
const open = computed(() => !!props.thread.status && !props.thread.closed)
const assignee = computed(() => props.thread.status?.assignee ?? '')
const busy = ref<Record<string, TabBusy>>({})
const targetId = ref<string | null>(null)
const sending = ref(false)
const error = ref<string | null>(null)
const folded = ref(false)
const scroller = ref<HTMLElement | null>(null)

const targets = computed(() =>
  resolveTargetsOf(props.project.id).map((target) => ({
    id: target.tabId,
    label: target.label,
    disabled: busy.value[target.tabId]?.busy
      ? (busy.value[target.tabId].reason ?? 'Busy')
      : undefined
  }))
)
const target = computed(() => targets.value.find((t) => t.id === targetId.value) ?? null)

const text = computed({
  get: () => review.value.message,
  set: (value: string) => setReviewMessage(props.project.id, props.scope, value)
})

const blocker = computed(() => {
  if (count.value === 0 && !review.value.message.trim()) {
    return 'Write a message or add a line comment first.'
  }
  if (open.value) return null
  if (!target.value) return 'Pick a tab to hand the conflict to.'
  return target.value.disabled ?? null
})

function labelOf(message: WorkgroupThreadMessage): string {
  if (message.author === 'human') return 'You'
  return shellTitles[message.tabId] ?? message.label
}

function linesOf(anchor: NonNullable<WorkgroupThreadMessage['anchor']>): string {
  return anchor.startLine === anchor.endLine
    ? `${anchor.path}:${anchor.startLine}`
    : `${anchor.path}:${anchor.startLine}-${anchor.endLine}`
}

// What a message is about, shown before its text: a note's or a line comment's lines, or the
// files a submit lists.
function contextOf(message: WorkgroupThreadMessage): string | null {
  if (message.kind === 'submit') {
    return message.files.length === 1
      ? `submitted ${message.files[0]}`
      : `submitted ${message.files.length} files: ${message.files.join(', ')}`
  }
  if (!message.anchor) return null
  return message.kind === 'note'
    ? `note on ${linesOf(message.anchor)}`
    : `on ${linesOf(message.anchor)}`
}

function partsOf(body: string): BodyPart[] {
  const members = assignee.value
    ? [{ tabId: assignee.value, label: shellTitles[assignee.value] ?? 'the model' }]
    : []
  return bodyParts(body, members)
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString()
}

function nearBottom(): boolean {
  const el = scroller.value
  return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

// New messages scroll into view unless the human is reading back.
watch(
  () => props.thread.messages.length,
  async () => {
    const stick = nearBottom()
    await nextTick()
    if (stick && scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight
  }
)

let pollTimer: ReturnType<typeof setInterval> | null = null

async function pollBusy(): Promise<void> {
  try {
    busy.value = await window.api.tabSessions.states(props.project.path)
  } catch {
    busy.value = {}
  }
}

// The picker only shows while no thread is open, so busy states are polled only then.
watch(
  open,
  (isOpen) => {
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
    if (isOpen) return
    void pollBusy()
    pollTimer = setInterval(pollBusy, BUSY_POLL_MS)
  },
  { immediate: true }
)

onMounted(async () => {
  await nextTick()
  if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
})

// Line comments stay drafts until Send, then go out as anchored messages, the text last.
function outgoing(): ConflictHumanMessage[] {
  const messages: ConflictHumanMessage[] = review.value.annotations.map((annotation) => ({
    body: annotation.comment,
    anchor: {
      path: annotation.anchor.path,
      startLine: annotation.anchor.startLine,
      endLine: annotation.anchor.endLine,
      quote: annotation.quote
    }
  }))
  const overall = review.value.message.trim()
  if (overall) messages.push({ body: overall, anchor: null })
  return messages
}

async function send(): Promise<void> {
  if (blocker.value || sending.value) return
  sending.value = true
  error.value = null
  try {
    if (!open.value) {
      if (!targetId.value) return
      const refused = await handConflictTo(props.project, targetId.value)
      if (refused) {
        error.value = refused
        return
      }
    }
    const refused = await postToConflict(props.project, outgoing())
    if (refused) error.value = refused
    else clearReview(props.project.id, props.scope)
  } finally {
    sending.value = false
  }
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
    event.preventDefault()
    void send()
  }
}
</script>

<template>
  <div class="thread-bar">
    <div class="bar-head">
      <button class="fold" :aria-expanded="!folded" @click="folded = !folded">
        <AppIcon class="fold-chevron" :class="{ open: !folded }" name="chevronRight" :size="9" />
        Conflict thread
      </button>
      <span v-if="open" class="head-note">
        with <span class="head-name">{{ shellTitles[assignee] ?? 'a tab that is closed' }}</span>
      </span>
      <span v-else class="head-note">Not handed to a tab yet</span>
      <span class="spacer" />
      <span class="count">{{ count === 1 ? '1 line comment' : `${count} line comments` }}</span>
    </div>

    <div v-if="!folded && open" ref="scroller" class="messages">
      <AppEmptyState v-if="thread.messages.length === 0" size="sm" title="No messages yet." />
      <article
        v-for="message in thread.messages"
        :key="message.id"
        class="message"
        :class="[message.author, message.kind]"
      >
        <header class="message-head">
          <span class="message-author">{{ labelOf(message) }}</span>
          <span v-if="contextOf(message)" class="message-context">{{ contextOf(message) }}</span>
          <time class="message-time">{{ formatTime(message.createdAt) }}</time>
        </header>
        <p v-if="message.anchor?.quote" class="message-quote">{{ message.anchor.quote }}</p>
        <p class="message-body">
          <template v-for="(part, index) in partsOf(message.body)" :key="index">
            <span :class="{ mention: part.mention }">{{ part.text }}</span>
          </template>
        </p>
      </article>
    </div>
    <p v-if="thread.error && !folded" class="error">{{ thread.error }}</p>

    <div class="composer">
      <AppTextarea
        v-model="text"
        class="composer-text"
        size="sm"
        resize="none"
        rows="2"
        spellcheck="false"
        :placeholder="open ? 'Reply in the conflict thread' : 'What should the model know?'"
        :disabled="sending"
        @keydown="onKeydown"
      />
      <div v-if="!open" class="picker">
        <AppSwitcher
          :items="targets"
          :active-id="targetId"
          placeholder="Hand to tab"
          empty-text="No Claude or Codex tab is open in this project."
          framed
          above
          @select="targetId = $event"
        />
      </div>
      <AppButton
        variant="primary"
        class="send"
        :disabled="!!blocker || sending"
        :title="
          blocker ?? (open ? 'Send to the conflict thread' : 'Hand the conflict over and send')
        "
        @click="send"
      >
        {{ sending ? 'Sending' : 'Send' }}
      </AppButton>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>

<style scoped>
.thread-bar {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-1);
  border-top: 1px solid var(--color-border-default);
  font-size: var(--text-xs);
}

.bar-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.fold {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  background: transparent;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  cursor: pointer;
}

.fold:hover {
  color: var(--color-fg-primary);
}

.fold-chevron {
  transition: transform var(--duration-fast) var(--ease-out);
}

.fold-chevron.open {
  transform: rotate(90deg);
}

.head-note {
  color: var(--color-fg-muted);
}

.head-name {
  color: var(--color-fg-primary);
}

.spacer {
  flex: 1;
}

.count {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
  font-weight: var(--weight-medium);
}

.messages {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-height: 200px;
  overflow-y: auto;
}

.message {
  padding: var(--space-1) var(--space-2);
  border-left: 2px solid var(--color-border-default);
}

/* Violet is the model's, as in the resolver; the human's own messages stay neutral. */
.message.tab {
  border-left-color: var(--color-violet);
}

.message-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--space-2);
}

.message-author {
  color: var(--color-fg-primary);
  font-weight: var(--weight-medium);
}

.message.tab .message-author {
  color: var(--color-violet);
}

.message-context {
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  word-break: break-all;
}

.message-time {
  color: var(--color-fg-muted);
}

.message-quote {
  margin-top: 2px;
  padding-left: var(--space-2);
  max-height: 60px;
  overflow: hidden;
  border-left: 1px solid var(--color-border-default);
  color: var(--color-fg-muted);
  font-family: var(--font-mono);
  white-space: pre;
}

.message-body {
  margin-top: 2px;
  color: var(--color-fg-primary);
  white-space: pre-wrap;
  word-break: break-word;
}

.message-body .mention {
  color: var(--color-fg-accent);
}

.composer {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
}

.composer-text {
  flex: 1;
  min-width: 160px;
}

.picker {
  width: 220px;
}

.error {
  color: var(--color-error);
}
</style>

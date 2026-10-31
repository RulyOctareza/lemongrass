<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { Text, type EditorState, type Extension } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { applyConflict } from '../../git'
import {
  dropDraft,
  registerWork,
  saveDraft,
  stagesOf,
  takeDraft,
  type ConflictDraft
} from '../../conflictDrafts'
import { conflictCommentExtension, refreshConflictComments } from '../../conflictAnnotations'
import { commentedLineRanges, isAnnotationChange } from '../../annotationGutter'
import {
  isNoteChange,
  notedLineRanges,
  noteCards,
  refreshNotes,
  type NoteMark
} from '../../noteCards'
import {
  applyText,
  buildMiddle,
  buildProposal,
  proposedIndexes,
  type MiddleProposal,
  collapseSpans,
  decide,
  decideAll,
  diskSegments,
  fromDisk,
  isCrlf,
  locateChunks,
  mapScroll,
  middleRanges,
  savedText,
  splitLines,
  stripChunks,
  toDisk,
  type ConflictSide,
  type ConflictTake,
  type SideDecision
} from '../../conflictModel'
import {
  acceptAction,
  acceptChunks,
  chunkAtCursor,
  chunkLines,
  focusLines,
  middleChunks,
  middleState,
  restoreMiddle,
  chunkReplacements,
  setFocusLines,
  showCurrent,
  showDecisions,
  snapshotMiddle,
  sideState
} from '../../conflictEditor'
import { hasConflictTab, type ProjectRef } from '../../workspace'
import type {
  ConflictPane,
  GitConflictChunk,
  GitConflictFile,
  GitConflictStage,
  GitLineRange,
  WorkgroupThreadMessage
} from '../../../../preload/types'
import AppButton from '../AppButton.vue'
import AppMenuButton from '../AppMenuButton.vue'
import AppModal from '../AppModal.vue'
import ConflictIcon from './ConflictIcon.vue'
import { CONFLICT_ICONS } from '../../conflictIcons'
import type { MenuItem } from '../AppContextMenu.vue'

type ConflictFile = Extract<GitConflictFile, { status: 'ok' }>

const props = withDefaults(
  defineProps<{
    project: ProjectRef
    path: string
    file: ConflictFile
    // The model's latest submit message that lists this file, which puts the middle in proposed
    // mode, and the model's notes on the file from the conflict thread.
    submitId?: number | null
    notes?: WorkgroupThreadMessage[]
  }>(),
  { submitId: null, notes: () => [] }
)

interface Loaded {
  chunks: GitConflictChunk[]
  finalNewline: boolean
  crlf: boolean
  doc: Text
  left: GitLineRange[]
  right: GitLineRange[]
}

const leftHost = ref<HTMLElement | null>(null)
const middleHost = ref<HTMLElement | null>(null)
const rightHost = ref<HTMLElement | null>(null)
const loaded = shallowRef<Loaded | null>(null)
const conflictIndexes = computed(() =>
  (loaded.value?.chunks ?? []).flatMap((chunk, index) => (chunk.kind === 'conflict' ? [index] : []))
)
const unresolved = ref(0)
// Proposals still waiting for the human.
const proposedLeft = ref(0)
const dirty = ref(false)
const current = ref<number | null>(null)
const applying = ref(false)
const error = ref<string | null>(null)
// A newer working file that arrived while the middle had unsaved edits.
const pendingReload = shallowRef<ConflictFile | null>(null)
const focus = ref(false)
// Flex weights of the three panes, changed by dragging the gutters between them.
const sizes = ref([1, 1, 1])
const panesEl = ref<HTMLElement | null>(null)
let workingSeen = props.file.working
type Source = {
  chunks: GitConflictChunk[]
  finalNewline: boolean
  proposal?: MiddleProposal | null
}
// What the middle was loaded from, which Reset goes back to.
let source: Source = { chunks: [], finalNewline: true }
// The chunk indexes holding the model's proposal in what is loaded.
let proposed = new Set<number>()
const proposalMode = ref(false)
// Set once the work is written, so unmounting keeps no draft of it.
let applied = false
let left: EditorView | null = null
let middle: EditorView | null = null
let right: EditorView | null = null

const position = computed(() => {
  const at = current.value === null ? -1 : conflictIndexes.value.indexOf(current.value)
  return at === -1 ? null : at + 1
})

const applyBlocked = computed((): string | null => {
  if (applying.value) return 'Applying'
  if (unresolved.value === 0) return null
  const left = unresolved.value === 1 ? '1 conflict left' : `${unresolved.value} conflicts left`
  const proposedPart =
    proposedLeft.value === 0
      ? ''
      : proposedLeft.value === unresolved.value
        ? ', proposed by the model'
        : `, ${proposedLeft.value} of them proposed`
  return `${left}${proposedPart}. Settle each one first.`
})

// The toolbar is status, not controls: a short counter, then what is left as plain text.
const counterText = computed(() => {
  const total = conflictIndexes.value.length
  if (total === 0) return 'No conflicts left'
  return position.value ? `${position.value} of ${total}` : `${total} conflicts`
})

const statusText = computed(() => {
  if (conflictIndexes.value.length === 0) return ''
  const open = unresolved.value - proposedLeft.value
  const parts = [
    ...(proposedLeft.value > 0 ? [`${proposedLeft.value} proposed`] : []),
    ...(open > 0 ? [`${open} open`] : [])
  ]
  return parts.length ? parts.join(', ') : 'all settled'
})

const statusTitle = computed(() => {
  const total = conflictIndexes.value.length
  if (total === 0) return 'No conflicts left in this file'
  const at = position.value ? `Conflict ${position.value} of ${total}. ` : ''
  return `${at}${unresolved.value} unresolved, ${proposedLeft.value} of them proposed by the model.`
})

// The whole-file actions replace every conflict in this file at once, so they sit behind the
// overflow icon, and confirm with a second click whenever a waiting proposal or the human's edits
// would go. They never reach past this file: the human still reads each file's code and proposal.
const menuItems = computed((): MenuItem[] => {
  const none = conflictIndexes.value.length === 0
  const atRisk = proposedLeft.value > 0 || dirty.value
  return [
    {
      key: 'left',
      label: `Take ${props.file.leftRef} in this file`,
      icon: CONFLICT_ICONS.takeLeft,
      disabled: none,
      confirm: atRisk
    },
    {
      key: 'right',
      label: `Take ${props.file.rightRef} in this file`,
      icon: CONFLICT_ICONS.takeRight,
      disabled: none,
      confirm: atRisk
    },
    {
      key: 'both',
      label: 'Take both in this file',
      icon: CONFLICT_ICONS.takeBoth,
      disabled: none,
      confirm: atRisk
    },
    {
      key: 'reset',
      label: proposalMode.value ? 'Reset to proposal' : 'Reset',
      icon: CONFLICT_ICONS.reset,
      title: proposalMode.value
        ? 'Put the middle back as the model proposed it'
        : 'Put the middle back as it was loaded',
      disabled: !dirty.value,
      confirm: true
    }
  ]
})

function onMenu(key: string): void {
  if (key === 'left' || key === 'right' || key === 'both') takeAll(key)
  else if (key === 'reset') reset()
}

// Below this toolbar width Accept all gives way; each proposed block still has its own Accept.
const NARROW_TOOLBAR = 320
const toolbarEl = ref<HTMLElement | null>(null)
const narrow = ref(false)
let toolbarObserver: ResizeObserver | null = null

function stageText(stage: GitConflictStage): string {
  return stage.state === 'text' ? stage.text.replace(/\r\n/g, '\n') : ''
}

// The middle starts from git's merge of the stages. After the file changed on disk it starts from
// the file instead, whose remaining markers become its conflicts. Once the model submitted this
// file, it starts from the model's file with git's conflicts mapped into it as proposals.
function chunksOf(file: ConflictFile, fromWorking: boolean): Source {
  if (props.submitId !== null && file.chunks && file.working !== null) {
    const built = buildProposal(file.chunks, file.working, file.workingChunks)
    if (built) return built
  }
  if (!fromWorking || file.working === null) {
    return { chunks: file.chunks ?? [], finalNewline: file.finalNewline }
  }
  const finalNewline = file.working.endsWith('\n')
  if (file.workingChunks) return { chunks: file.workingChunks, finalNewline }
  const lines = splitLines(file.working)
  return {
    chunks: [{ kind: 'clean', lines, disk: { start: 1, end: lines.length } }],
    finalNewline
  }
}

function middleLineRanges(state: EditorState): GitLineRange[] {
  const chunks = loaded.value?.chunks ?? []
  const conflicts = state.field(middleChunks).map((chunk) => chunkLines(state.doc, chunk))
  return middleRanges(chunks, conflicts, state.doc.lines)
}

function rangesOf(pane: ConflictPane): GitLineRange[] {
  if (pane === 'middle') return middle ? middleLineRanges(middle.state) : []
  return loaded.value?.[pane] ?? []
}

function segmentsOf(pane: ConflictPane): ReturnType<typeof diskSegments> {
  return diskSegments(loaded.value?.chunks ?? [], rangesOf(pane), pane, proposed)
}

// The model's notes, shown only on its proposal, since their lines are the model's file.
function notesIn(state: EditorState): NoteMark[] {
  if (!proposalMode.value || !loaded.value) return []
  const segments = diskSegments(loaded.value.chunks, middleLineRanges(state), 'middle', proposed)
  return props.notes.flatMap((note) => {
    if (!note.anchor) return []
    const range = fromDisk(segments, note.anchor.startLine, note.anchor.endLine)
    return range ? [{ id: note.id, ...range, author: note.label, body: note.body }] : []
  })
}

function reviewExtension(pane: ConflictPane): Extension {
  return conflictCommentExtension({
    project: props.project,
    path: props.path,
    pane,
    operation: props.file.operation,
    leftRef: props.file.leftRef,
    rightRef: props.file.rightRef,
    toDisk: (fromLine, toLine) => toDisk(segmentsOf(pane), fromLine, toLine),
    fromDisk: (startLine, endLine) => fromDisk(segmentsOf(pane), startLine, endLine),
    chunks: () =>
      conflictIndexes.value
        .map((index) => rangesOf(pane)[index])
        .filter((range) => range && range.end >= range.start)
        .map((range) => ({ fromLine: range.start, toLine: range.end }))
  })
}

// Focus hides clean lines away from the conflicts, but never a line that carries a comment.
function focusExtension(pane: ConflictPane): Extension {
  return focusLines(
    (state) => {
      const chunks = loaded.value?.chunks ?? []
      const ranges = pane === 'middle' ? middleLineRanges(state) : (loaded.value?.[pane] ?? [])
      const clean = chunks.flatMap((chunk, index) =>
        chunk.kind === 'clean' ? [ranges[index]] : []
      )
      const keep = [...commentedLineRanges(state), ...notedLineRanges(state)].map((range) => ({
        start: range.fromLine,
        end: range.toLine
      }))
      return collapseSpans(clean, keep, state.doc.lines)
    },
    (tr) => isAnnotationChange(tr) || isNoteChange(tr)
  )
}

function toggleFocus(): void {
  focus.value = !focus.value
  for (const view of [left, middle, right]) if (view) setFocusLines(view, focus.value)
}

function onMiddleUpdate(state: EditorState): void {
  const at = chunkAtCursor(state)
  if (at !== null) current.value = at
  const chunks = state.field(middleChunks)
  unresolved.value = chunks.filter((chunk) => !chunk.resolved).length
  const waiting = chunks.filter((chunk) => chunk.proposed && !chunk.resolved)
  proposedLeft.value = waiting.length
  // An accepted proposal changes no text but is still work a reload would lose.
  dirty.value =
    loaded.value !== null &&
    (!state.doc.eq(loaded.value.doc) || (proposalMode.value && chunks.some((c) => c.resolved)))
  pushDecisions(chunks)
}

let shownDecisions = ''

// The side panes draw the middle's decisions on their chevrons, so they hear about every change.
function pushDecisions(chunks: { index: number; left: SideDecision; right: SideDecision }[]): void {
  const key = JSON.stringify(chunks.map(({ index, left, right }) => [index, left, right]))
  if (key === shownDecisions || !left || !right) return
  shownDecisions = key
  for (const [view, side] of [
    [left, 'left'],
    [right, 'right']
  ] as const) {
    showDecisions(view, Object.fromEntries(chunks.map((chunk) => [chunk.index, chunk[side]])))
  }
}

// A draft restores the middle as it was left, decisions included; otherwise it is built from raw.
function load(file: ConflictFile, raw: Source, draft: ConflictDraft | null = null): void {
  source = raw
  proposed = proposedIndexes(raw.proposal ?? null)
  proposalMode.value = !!raw.proposal
  const crlf = isCrlf(raw.chunks)
  const chunks = stripChunks(raw.chunks)
  const built = buildMiddle(chunks, raw.proposal ?? null)
  const leftText = stageText(file.left)
  const rightText = stageText(file.right)
  const empty = chunks.map(() => ({ start: 1, end: 0 }))
  const located = {
    left: locateChunks(chunks, 'left', leftText) ?? empty,
    right: locateChunks(chunks, 'right', rightText) ?? empty
  }
  const sides = (pane: 'left' | 'right'): { index: number; range: GitLineRange }[] =>
    chunks.flatMap((chunk, index) =>
      chunk.kind === 'conflict' ? [{ index, range: located[pane][index] }] : []
    )
  const middleExtras = [
    reviewExtension('middle'),
    focusExtension('middle'),
    noteCards(notesIn),
    acceptAction((index) => accept([index]))
  ]
  const middleDoc = draft
    ? restoreMiddle(draft.middle, onMiddleUpdate, middleExtras)
    : middleState(built.text, built.chunks, onMiddleUpdate, middleExtras, built.edits)
  loaded.value = {
    chunks,
    finalNewline: raw.finalNewline,
    crlf,
    doc: draft ? Text.of(draft.loadedText.split('\n')) : middleDoc.doc,
    left: located.left,
    right: located.right
  }
  const leftDoc = sideState(
    leftText,
    sides('left'),
    'left',
    file.leftRef,
    (index, d) => decideSide(index, 'left', d),
    [reviewExtension('left'), focusExtension('left')]
  )
  const rightDoc = sideState(
    rightText,
    sides('right'),
    'right',
    file.rightRef,
    (index, d) => decideSide(index, 'right', d),
    [reviewExtension('right'), focusExtension('right')]
  )
  if (left && middle && right) {
    left.setState(leftDoc)
    middle.setState(middleDoc)
    right.setState(rightDoc)
    for (const view of [left, middle, right]) refreshConflictComments(view)
  } else {
    left = new EditorView({ state: leftDoc, parent: leftHost.value! })
    middle = new EditorView({ state: middleDoc, parent: middleHost.value! })
    right = new EditorView({ state: rightDoc, parent: rightHost.value! })
    for (const view of [left, middle, right]) {
      view.scrollDOM.addEventListener('scroll', () => syncScroll(view))
    }
  }
  if (focus.value) for (const view of [left, middle, right]) setFocusLines(view, true)
  current.value = null
  error.value = null
  shownDecisions = ''
  onMiddleUpdate(middleDoc)
  if (draft) {
    current.value = draft.current
    middle.dispatch({
      effects: EditorView.scrollIntoView(middleDoc.selection.main.head, { y: 'center' })
    })
  } else if (built.chunks.length > 0) goTo(built.chunks[0].index)
}

watch(current, (index) => {
  for (const view of [left, right]) if (view) showCurrent(view, index)
  // The middle's own listener sets current, and it may not dispatch while it is updating.
  queueMicrotask(() => middle && showCurrent(middle, index))
})

function goTo(index: number): void {
  if (!middle) return
  const chunk = middle.state.field(middleChunks).find((c) => c.index === index)
  if (!chunk) return
  current.value = index
  middle.dispatch({
    selection: { anchor: chunk.from },
    effects: EditorView.scrollIntoView(chunk.from, { y: 'center' })
  })
}

function step(direction: 1 | -1): void {
  const list = conflictIndexes.value
  if (list.length === 0) return
  const at = current.value === null ? -1 : list.indexOf(current.value)
  const next = at === -1 ? (direction === 1 ? 0 : list.length - 1) : at + direction
  goTo(list[(next + list.length) % list.length])
}

function conflictAt(index: number): Extract<GitConflictChunk, { kind: 'conflict' }> | null {
  const chunk = loaded.value?.chunks[index]
  return chunk?.kind === 'conflict' ? chunk : null
}

// A chevron or cross in a side pane decides that side of one conflict.
function decideSide(index: number, side: ConflictSide, decision: 'taken' | 'rejected'): void {
  const chunk = middle?.state.field(middleChunks).find((c) => c.index === index)
  const conflict = conflictAt(index)
  if (!middle || !chunk || !conflict) return
  const spec = chunkReplacements(middle.state, [decide(chunk, conflict, side, decision)])
  if (!spec) return
  current.value = index
  middle.dispatch(spec)
}

// The toolbar's takes decide every conflict in the file at once.
function takeAll(take: ConflictTake): void {
  if (!middle) return
  const updates = middle.state.field(middleChunks).flatMap((chunk) => {
    const conflict = conflictAt(chunk.index)
    return conflict ? [decideAll(chunk, conflict, take)] : []
  })
  const spec = chunkReplacements(middle.state, updates)
  if (spec) middle.dispatch(spec)
}

// Accept keeps the model's text for the current conflict, Accept all for every proposal left.
// Accept keeps the model's text: one block from its own Accept, or every proposal left.
function accept(indexes: number[] | 'all'): void {
  if (!middle) return
  const chosen =
    indexes === 'all' ? middle.state.field(middleChunks).map((chunk) => chunk.index) : indexes
  const spec = acceptChunks(middle.state, chosen)
  if (spec) middle.dispatch(spec)
}

// Dragging the gutter after pane `at` moves width between it and the next pane, each keeping at
// least 15 percent, so a narrow window can still give one pane room.
function startResize(at: number, event: PointerEvent): void {
  const width = panesEl.value?.clientWidth ?? 0
  if (!width) return
  const total = sizes.value.reduce((sum, size) => sum + size, 0)
  const start = [...sizes.value]
  const startX = event.clientX
  const min = total * 0.15
  const target = event.currentTarget as HTMLElement
  target.setPointerCapture(event.pointerId)
  const move = (e: PointerEvent): void => {
    const pair = start[at] + start[at + 1]
    const next = Math.min(
      pair - min,
      Math.max(min, start[at] + ((e.clientX - startX) / width) * total)
    )
    const updated = [...start]
    updated[at] = next
    updated[at + 1] = pair - next
    sizes.value = updated
  }
  const end = (): void => {
    target.removeEventListener('pointermove', move)
    target.removeEventListener('pointerup', end)
    target.removeEventListener('pointercancel', end)
  }
  target.addEventListener('pointermove', move)
  target.addEventListener('pointerup', end)
  target.addEventListener('pointercancel', end)
}

function reset(): void {
  load(props.file, source)
}

async function apply(): Promise<void> {
  if (!middle || !loaded.value || applyBlocked.value) return
  applying.value = true
  const content = applyText(
    middle.state.doc.toString(),
    loaded.value.finalNewline,
    loaded.value.crlf
  )
  error.value = await applyConflict(props.project, props.path, content)
  if (!error.value) {
    dirty.value = false
    applied = true
    dropDraft(props.project.id, props.path)
  }
  applying.value = false
}

// A model or a shell changed the working file. With nothing unsaved in the middle it reloads at
// once, otherwise the human chooses, so unsaved edits never silently replace the model's.
function onFileChange(file: ConflictFile): void {
  if (file.working === workingSeen) return
  if (dirty.value) {
    pendingReload.value = file
    return
  }
  workingSeen = file.working
  load(file, chunksOf(file, true))
}

watch(() => props.file, onFileChange)

// A new submission reloads the proposal, or asks first when the middle holds the human's work.
watch(
  () => props.submitId,
  () => {
    if (dirty.value) {
      pendingReload.value = props.file
      return
    }
    workingSeen = props.file.working
    load(props.file, chunksOf(props.file, true))
  }
)

watch(
  () => props.notes,
  () => {
    if (middle) refreshNotes(middle)
  }
)

function reloadPending(): void {
  const file = pendingReload.value
  pendingReload.value = null
  if (!file) return
  workingSeen = file.working
  load(file, chunksOf(file, true))
}

function keepEdits(): void {
  workingSeen = pendingReload.value?.working ?? workingSeen
  pendingReload.value = null
}

// Scroll position as a fractional line boundary, 0 being the top of the first line.
function scrollLine(view: EditorView): number {
  const top = view.scrollDOM.scrollTop - view.documentPadding.top
  const block = view.lineBlockAtHeight(Math.max(0, top))
  const line = view.state.doc.lineAt(block.from).number
  const fraction = block.height > 0 ? (top - block.top) / block.height : 0
  return line - 1 + Math.min(1, Math.max(0, fraction))
}

function scrollTopOf(view: EditorView, position: number): number {
  const doc = view.state.doc
  const line = Math.min(doc.lines, Math.max(1, Math.floor(position) + 1))
  const block = view.lineBlockAt(doc.line(line).from)
  return block.top + (position - (line - 1)) * block.height + view.documentPadding.top
}

// Panes moved by the sync skip their own next scroll event, so the sync never echoes back.
const synced = new Set<EditorView>()

function syncScroll(from: EditorView): void {
  if (synced.delete(from)) return
  const paneOf = (view: EditorView): ConflictPane =>
    view === left ? 'left' : view === right ? 'right' : 'middle'
  const position = scrollLine(from)
  for (const to of [left, middle, right]) {
    if (!to || to === from) continue
    const mapped = mapScroll(
      rangesOf(paneOf(from)),
      rangesOf(paneOf(to)),
      from.state.doc.lines,
      to.state.doc.lines,
      position
    )
    const before = to.scrollDOM.scrollTop
    to.scrollDOM.scrollTop = scrollTopOf(to, mapped)
    if (Math.abs(to.scrollDOM.scrollTop - before) >= 1) synced.add(to)
  }
}

const refs = {
  operation: props.file.operation,
  leftRef: props.file.leftRef,
  rightRef: props.file.rightRef
}

// A review saves this tab's work to disk first, but only once the middle differs from what it
// loaded.
const unregisterWork = registerWork(props.project.id, props.path, () => {
  if (!middle || !loaded.value || middle.state.doc.eq(loaded.value.doc)) return null
  const saved = savedText(
    middle.state.doc.toString(),
    middle.state.field(middleChunks),
    loaded.value.chunks,
    refs,
    loaded.value.finalNewline,
    loaded.value.crlf
  )
  return { ...saved, markSeen: (text: string) => (workingSeen = text) }
})

onMounted(() => {
  toolbarObserver = new ResizeObserver(([entry]) => {
    narrow.value = entry.contentRect.width < NARROW_TOOLBAR
  })
  if (toolbarEl.value) toolbarObserver.observe(toolbarEl.value)
  const draft = takeDraft(props.project.id, props.path, stagesOf(props.file))
  if (draft) {
    workingSeen = draft.workingSeen
    focus.value = draft.focus
    sizes.value = draft.sizes
  }
  load(props.file, draft ?? chunksOf(props.file, false), draft)
  // A working file that changed while the tab was away reloads or asks, as it would have.
  if (draft && props.file.working !== workingSeen) onFileChange(props.file)
})

// The tab is unmounted when its project is switched away or it moves to another pane; it keeps
// its work for the next mount unless it was closed or applied.
onBeforeUnmount(() => {
  toolbarObserver?.disconnect()
  unregisterWork()
  if (middle && loaded.value && !applied && hasConflictTab(props.project.id, props.path)) {
    saveDraft(props.project.id, props.path, {
      stages: stagesOf(props.file),
      chunks: source.chunks,
      finalNewline: source.finalNewline,
      proposal: source.proposal ?? null,
      loadedText: loaded.value.doc.toString(),
      refs,
      middle: snapshotMiddle(middle.state),
      workingSeen,
      focus: focus.value,
      sizes: sizes.value,
      current: current.value
    })
  }
  for (const view of [left, middle, right]) view?.destroy()
})
</script>

<template>
  <div class="conflict-merge">
    <!-- Apply is the only button with a frame: navigation and Focus are bare icons, what is left
         is plain text, and the whole-file takes wait behind the overflow icon. -->
    <div ref="toolbarEl" class="toolbar">
      <div class="nav">
        <button
          type="button"
          class="bar-icon"
          title="Previous conflict"
          aria-label="Previous conflict"
          :disabled="!conflictIndexes.length"
          @click="step(-1)"
        >
          <ConflictIcon name="prev" />
        </button>
        <span class="counter">{{ counterText }}</span>
        <button
          type="button"
          class="bar-icon"
          title="Next conflict"
          aria-label="Next conflict"
          :disabled="!conflictIndexes.length"
          @click="step(1)"
        >
          <ConflictIcon name="next" />
        </button>
      </div>
      <span v-if="statusText" class="status" :title="statusTitle">{{ statusText }}</span>
      <button
        v-if="proposalMode && proposedLeft > 0 && !narrow"
        type="button"
        class="accept-all"
        title="Keep the model's text for every conflict it proposed in this file"
        @click="accept('all')"
      >
        Accept all
      </button>
      <span class="spacer" />
      <button
        type="button"
        class="bar-icon"
        :class="{ on: focus }"
        :aria-pressed="focus"
        :title="focus ? 'Show every line' : 'Hide the lines that need no merging'"
        :aria-label="focus ? 'Show every line' : 'Hide the lines that need no merging'"
        @click="toggleFocus"
      >
        <ConflictIcon name="focus" />
      </button>
      <AppMenuButton :items="menuItems" label="Whole-file actions" @select="onMenu">
        <ConflictIcon name="more" />
      </AppMenuButton>
      <AppButton
        variant="primary"
        class="apply"
        :title="applyBlocked ?? 'Write the middle to the file and mark it resolved'"
        :disabled="applyBlocked !== null"
        @click="apply"
      >
        Apply
      </AppButton>
    </div>
    <p v-if="error" class="merge-error">{{ error }}</p>
    <div ref="panesEl" class="panes">
      <section class="pane" :style="{ flexGrow: sizes[0] }">
        <header class="pane-head" :title="file.leftRef">
          <span class="side ref">{{ file.leftRef }}</span>
        </header>
        <div ref="leftHost" class="pane-body" />
      </section>
      <div class="gutter" @pointerdown="startResize(0, $event)" />
      <section class="pane middle" :style="{ flexGrow: sizes[1] }">
        <header class="pane-head">
          <span class="side">Result</span>
          <span v-if="dirty" class="ref">Unsaved</span>
        </header>
        <div ref="middleHost" class="pane-body" />
      </section>
      <div class="gutter" @pointerdown="startResize(1, $event)" />
      <section class="pane" :style="{ flexGrow: sizes[2] }">
        <header class="pane-head" :title="file.rightRef">
          <span class="side ref">{{ file.rightRef }}</span>
        </header>
        <div ref="rightHost" class="pane-body" />
      </section>
    </div>

    <AppModal v-if="pendingReload" :title="`${path} changed on disk`" @close="keepEdits">
      <p class="reload-note">
        The file changed while the middle pane has unsaved edits. Reloading drops those edits.
      </p>
      <template #footer>
        <AppButton @click="keepEdits">Keep my edits</AppButton>
        <AppButton variant="primary" @click="reloadPending">Reload</AppButton>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.conflict-merge {
  position: relative;
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border-subtle);
}

.nav {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 2px;
}

.counter {
  min-width: 4ch;
  font-size: var(--text-xs);
  color: var(--color-fg-secondary);
  text-align: center;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

/* Status, not a control: the only part of the bar that gives way when it is narrow. */
.status {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--text-xs);
  color: var(--color-fg-muted);
}

/* Bare icons, as in the git panel: muted, accent on hover and keyboard focus, lit while on. */
.bar-icon {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  padding: 3px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  cursor: pointer;
}

.bar-icon:hover:not(:disabled),
.bar-icon:focus-visible,
.bar-icon.on {
  color: var(--color-fg-accent);
}

.bar-icon:focus-visible {
  outline: 1px solid var(--color-amber);
}

.bar-icon:disabled {
  color: var(--color-fg-disabled);
  cursor: default;
}

/* Frameless jade text, the go action for every proposal at once. */
.accept-all {
  flex-shrink: 0;
  padding: 1px 4px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-jade);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  white-space: nowrap;
  cursor: pointer;
}

.accept-all:hover,
.accept-all:focus-visible {
  background: var(--color-jade-muted);
}

.accept-all:focus-visible {
  outline: 1px solid var(--color-amber);
}

.toolbar .apply {
  flex-shrink: 0;
}

.spacer {
  flex: 1;
}

.merge-error {
  margin: 0;
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-xs);
  color: var(--color-error);
}

.panes {
  display: flex;
  flex: 1;
  min-height: 0;
}

.pane {
  display: flex;
  flex-direction: column;
  flex: 1 1 0;
  min-width: 0;
}

.gutter {
  flex-shrink: 0;
  width: 4px;
  cursor: col-resize;
  background: var(--color-border-subtle);
  transition: background var(--duration-fast) var(--ease-out);
}

.gutter:hover,
.gutter:active {
  background: var(--color-amber);
}

.pane-head {
  display: flex;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-3);
  font-size: var(--text-xs);
  border-bottom: 1px solid var(--color-border-subtle);
  white-space: nowrap;
  overflow: hidden;
}

.side {
  color: var(--color-fg-primary);
  font-weight: var(--weight-medium);
}

.ref {
  color: var(--color-fg-muted);
  overflow: hidden;
  text-overflow: ellipsis;
}

.side.ref {
  color: var(--color-fg-primary);
}

.pane-body {
  flex: 1;
  min-height: 0;
}

.reload-note {
  margin: 0;
  font-size: var(--text-sm);
  color: var(--color-fg-secondary);
}
</style>

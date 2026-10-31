// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import {
  EditorState,
  Facet,
  RangeSetBuilder,
  StateEffect,
  StateField,
  type Extension,
  type Text,
  type Transaction,
  type TransactionSpec
} from '@codemirror/state'
import {
  Decoration,
  EditorView,
  GutterMarker,
  WidgetType,
  gutter,
  drawSelection,
  keymap,
  lineNumbers,
  type DecorationSet
} from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, invertedEffects } from '@codemirror/commands'
import type { GitLineRange } from '../../preload/types'
import type { ConflictSide, MiddleChunk, SideDecision } from './conflictModel'

// A conflict's place in a side pane, which never changes since the side panes are read only.
export interface SideChunk {
  index: number
  range: GitLineRange
}

const setChunk = StateEffect.define<MiddleChunk>()
const setCurrent = StateEffect.define<number | null>()

// The middle's conflicts follow every edit. An edit that touches a conflict resolves it, and an
// empty conflict takes text typed at its position, since there is nowhere else for it to go.
export const middleChunks = StateField.define<MiddleChunk[]>({
  create: () => [],
  // Plain data, so a saved middle keeps its decisions.
  toJSON: (chunks) => chunks,
  fromJSON: (json) => json as MiddleChunk[],
  update(chunks, tr) {
    let next = chunks
    if (tr.docChanged) {
      next = chunks.map((chunk) => {
        let touched = false
        tr.changes.iterChangedRanges((fromA, toA) => {
          if (
            fromA === toA
              ? fromA >= chunk.from && fromA < chunk.to
              : fromA < chunk.to && toA > chunk.from
          ) {
            touched = true
          }
          if (chunk.from === chunk.to && fromA <= chunk.from && toA >= chunk.from) touched = true
        })
        const from = tr.changes.mapPos(chunk.from, -1)
        const to = tr.changes.mapPos(chunk.to, chunk.from === chunk.to ? 1 : -1)
        // A hand edit settles a proposal as much as an unresolved conflict.
        return {
          ...chunk,
          from,
          to: Math.max(from, to),
          resolved: chunk.resolved || touched,
          ...(chunk.proposed && touched ? { proposed: false } : {})
        }
      })
    }
    for (const effect of tr.effects) {
      if (effect.is(setChunk)) {
        next = next.map((chunk) => (chunk.index === effect.value.index ? effect.value : chunk))
      }
    }
    return next
  }
})

// Undoing a take puts the conflict back as it was, unresolved included.
const undoChunks = invertedEffects.of((tr) => {
  const before = tr.startState.field(middleChunks, false)
  if (!before) return []
  return tr.effects.flatMap((effect) => {
    if (!effect.is(setChunk)) return []
    const old = before.find((chunk) => chunk.index === effect.value.index)
    return old ? [setChunk.of(old)] : []
  })
})

// Clean lines the model changed in its submission, as character ranges that follow every edit, so
// the human sees what the model touched outside the conflicts.
const modelEdits = StateField.define<{ from: number; to: number }[]>({
  create: () => [],
  toJSON: (ranges) => ranges,
  fromJSON: (json) => json as { from: number; to: number }[],
  update(ranges, tr) {
    if (!tr.docChanged) return ranges
    return ranges
      .map((range) => ({
        from: tr.changes.mapPos(range.from, 1),
        to: tr.changes.mapPos(range.to, -1)
      }))
      .filter((range) => range.to > range.from)
  }
})

const modelEditDecorations = EditorView.decorations.compute(['doc', modelEdits], (state) =>
  lineClasses(
    state.doc,
    state.field(modelEdits).map((range) => ({ ...range, className: 'cm-model-edit' }))
  )
)

// Accept keeps the model's text for these conflicts and resolves them. Only the decisions change,
// through the same effect as a take, so undo brings the proposals back.
export function acceptChunks(state: EditorState, indexes: number[]): TransactionSpec | null {
  const effects = state
    .field(middleChunks)
    .filter((chunk) => indexes.includes(chunk.index) && chunk.proposed && !chunk.resolved)
    .map((chunk) => setChunk.of({ ...chunk, proposed: false, resolved: true }))
  return effects.length ? { effects } : null
}

const currentChunk = StateField.define<number | null>({
  create: () => null,
  update(value, tr) {
    for (const effect of tr.effects) if (effect.is(setCurrent)) value = effect.value
    return value
  }
})

const sideChunks = StateField.define<SideChunk[]>({
  create: () => [],
  update: (value) => value
})

const setDecisions = StateEffect.define<Record<number, SideDecision>>()

// What the middle decided for this pane's side of each conflict, pushed in from the middle.
const sideDecisions = StateField.define<Record<number, SideDecision>>({
  create: () => ({}),
  update(value, tr) {
    for (const effect of tr.effects) if (effect.is(setDecisions)) value = effect.value
    return value
  }
})

export type OnDecide = (index: number, decision: 'taken' | 'rejected') => void

const CHEVRONS = {
  left: 'M3 3.5l3.5 3.5L3 10.5M7.5 3.5L11 7l-3.5 3.5',
  right: 'M11 3.5L7.5 7l3.5 3.5M6.5 3.5L3 7l3.5 3.5'
}
const CROSS = 'M4 4l6 6M10 4l-6 6'

function iconButton(className: string, path: string, label: string, action: string): HTMLElement {
  const button = document.createElement('button')
  button.className = `cm-decide-button ${className}`
  button.title = label
  button.setAttribute('aria-label', label)
  button.dataset.decide = action
  button.innerHTML = `<svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="${path}"/></svg>`
  return button
}

// The chevron takes this pane's side into the middle and the cross rejects it. The decided one
// stays lit.
// branch names the side in titles, since left and right are never shown to the human.
class DecideMarker extends GutterMarker {
  readonly side: ConflictSide
  readonly branch: string
  readonly decision: SideDecision

  constructor(side: ConflictSide, branch: string, decision: SideDecision) {
    super()
    this.side = side
    this.branch = branch
    this.decision = decision
  }

  eq(other: DecideMarker): boolean {
    return (
      other.side === this.side && other.branch === this.branch && other.decision === this.decision
    )
  }

  toDOM(): Node {
    const dom = document.createElement('span')
    dom.className = 'cm-decide'
    const lit = (own: SideDecision): string =>
      this.decision === 'open' ? '' : this.decision === own ? ' on' : ' off'
    const take = iconButton(
      `take${lit('taken')}`,
      CHEVRONS[this.side],
      `Take ${this.branch}`,
      'taken'
    )
    const reject = iconButton(
      `reject${lit('rejected')}`,
      CROSS,
      `Reject ${this.branch}`,
      'rejected'
    )
    // The chevron points into the middle, so it sits on the middle's side of the cross.
    if (this.side === 'left') dom.append(reject, take)
    else dom.append(take, reject)
    return dom
  }
}

// The conflict whose first line is this one, counting an empty side at the line after it.
function sideChunkAt(state: EditorState, lineNumber: number): SideChunk | undefined {
  return state
    .field(sideChunks)
    .find(({ range }) => Math.min(range.start, state.doc.lines) === lineNumber)
}

// The decision gutter sits next to the middle: after the text in the left pane, before it in the
// right pane, so the two side panes mirror each other.
function decisionGutter(side: ConflictSide, branch: string, onDecide: OnDecide): Extension {
  return gutter({
    class: 'cm-conflict-decide',
    side: side === 'left' ? 'after' : 'before',
    lineMarker(view, line) {
      const chunk = sideChunkAt(view.state, view.state.doc.lineAt(line.from).number)
      if (!chunk) return null
      return new DecideMarker(side, branch, view.state.field(sideDecisions)[chunk.index] ?? 'open')
    },
    lineMarkerChange: (update) =>
      update.startState.field(sideDecisions) !== update.state.field(sideDecisions),
    initialSpacer: () => new DecideMarker(side, branch, 'open'),
    domEventHandlers: {
      mousedown(view, line, event) {
        const action = (event.target as HTMLElement).closest('[data-decide]')
        if (!action) return false
        const chunk = sideChunkAt(view.state, view.state.doc.lineAt(line.from).number)
        if (chunk) onDecide(chunk.index, action.getAttribute('data-decide') as 'taken' | 'rejected')
        event.preventDefault()
        return true
      }
    }
  })
}

class LineNumberMarker extends GutterMarker {
  readonly number: number

  constructor(number: number) {
    super()
    this.number = number
  }

  eq(other: LineNumberMarker): boolean {
    return other.number === this.number
  }

  toDOM(): Node {
    return document.createTextNode(String(this.number))
  }
}

// lineNumbers() can only sit before the text, and the left pane shows its numbers after it, next
// to the middle. The cm-lineNumbers class gives it the same look.
const lineNumbersAfter = gutter({
  class: 'cm-lineNumbers cm-lineNumbers-after',
  side: 'after',
  lineMarker: (view, line) => new LineNumberMarker(view.state.doc.lineAt(line.from).number),
  lineMarkerChange: (update) => update.docChanged,
  initialSpacer: (view) => new LineNumberMarker(view.state.doc.lines)
})

type OnAccept = (index: number) => void

// What Accept on a proposed block runs, given by the resolver that owns the middle.
const acceptHandler = Facet.define<OnAccept, OnAccept | null>({
  combine: (values) => values[0] ?? null
})

export function acceptAction(onAccept: OnAccept): Extension {
  return acceptHandler.of(onAccept)
}

// The solid jade Accept on a proposed block, so the decision sits where the eye already is.
function acceptButton(index: number, onAccept: OnAccept): HTMLElement {
  const button = document.createElement('button')
  button.type = 'button'
  button.className = 'cm-accept'
  button.textContent = 'Accept'
  button.title = "Keep the model's text for this conflict"
  // mousedown, not click, so the editor never moves its cursor or selection under the press.
  button.addEventListener('mousedown', (event) => {
    event.preventDefault()
    onAccept(index)
  })
  return button
}

class AcceptWidget extends WidgetType {
  readonly index: number
  readonly onAccept: OnAccept

  constructor(index: number, onAccept: OnAccept) {
    super()
    this.index = index
    this.onAccept = onAccept
  }

  eq(other: AcceptWidget): boolean {
    return other.index === this.index && other.onAccept === this.onAccept
  }

  toDOM(): HTMLElement {
    return acceptButton(this.index, this.onAccept)
  }

  ignoreEvent(): boolean {
    return true
  }
}

class EmptyChunkWidget extends WidgetType {
  readonly index: number
  readonly resolved: boolean
  readonly look: ChunkLook
  readonly onAccept: OnAccept | null

  constructor(index: number, resolved: boolean, look: ChunkLook, onAccept: OnAccept | null) {
    super()
    this.index = index
    this.resolved = resolved
    this.look = look
    this.onAccept = onAccept
  }

  eq(other: EmptyChunkWidget): boolean {
    return (
      other.index === this.index &&
      other.resolved === this.resolved &&
      other.look === this.look &&
      other.onAccept === this.onAccept
    )
  }

  toDOM(): HTMLElement {
    const el = document.createElement('div')
    el.className = `cm-conflict-empty ${this.look}`
    el.textContent = this.resolved
      ? 'Resolved as empty'
      : this.look === 'proposed'
        ? 'The model removed these lines.'
        : 'No lines here. Decide each side or type.'
    if (this.look === 'proposed' && this.onAccept) {
      el.append(acceptButton(this.index, this.onAccept))
    }
    return el
  }

  ignoreEvent(): boolean {
    return true
  }
}

class HiddenLinesWidget extends WidgetType {
  readonly count: number

  constructor(count: number) {
    super()
    this.count = count
  }

  eq(other: HiddenLinesWidget): boolean {
    return other.count === this.count
  }

  toDOM(): HTMLElement {
    const dom = document.createElement('div')
    dom.className = 'cm-conflict-hidden'
    dom.textContent = `${this.count} lines hidden`
    return dom
  }
}

const setFocus = StateEffect.define<boolean>()

// Focus hides the clean lines far from any conflict. spans gives the lines to hide in a state, and
// is asked again on every edit and on every change for which recompute says yes.
export function focusLines(
  spans: (state: EditorState) => GitLineRange[],
  recompute: (tr: Transaction) => boolean
): Extension {
  const build = (state: EditorState): DecorationSet =>
    Decoration.set(
      spans(state)
        .filter((span) => span.start >= 1 && span.end <= state.doc.lines && span.end > span.start)
        .map((span) =>
          Decoration.replace({
            widget: new HiddenLinesWidget(span.end - span.start + 1),
            block: true
          }).range(state.doc.line(span.start).from, state.doc.line(span.end).to)
        )
    )
  return StateField.define<{ on: boolean; lines: DecorationSet }>({
    create: () => ({ on: false, lines: Decoration.none }),
    update(value, tr) {
      let on = value.on
      for (const effect of tr.effects) if (effect.is(setFocus)) on = effect.value
      if (!on) return value.on ? { on, lines: Decoration.none } : value
      if (on === value.on && !tr.docChanged && !recompute(tr)) return value
      return { on, lines: build(tr.state) }
    },
    provide: (field) => EditorView.decorations.from(field, (value) => value.lines)
  })
}

export function setFocusLines(view: EditorView, on: boolean): void {
  view.dispatch({ effects: setFocus.of(on) })
}

function lineClasses(
  doc: Text,
  ranges: { from: number; to: number; className: string }[]
): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()
  for (const range of ranges) {
    if (range.from === range.to) continue
    const first = doc.lineAt(range.from).number
    const last = doc.lineAt(range.to - 1).number
    for (let line = first; line <= last; line++) {
      builder.add(
        doc.line(line).from,
        doc.line(line).from,
        Decoration.line({ class: range.className })
      )
    }
  }
  return builder.finish()
}

type ChunkLook = 'open' | 'picked' | 'proposed'

// A middle conflict is red until it holds something chosen: a taken side, a hand edit, or both
// sides decided. The model's proposal is violet until the human settles it.
function lookOf(chunk: MiddleChunk): ChunkLook {
  if (chunk.proposed && !chunk.resolved) return 'proposed'
  return chunk.resolved || chunk.taken.length > 0 ? 'picked' : 'open'
}

function chunkClass(chunk: MiddleChunk, current: number | null): string {
  const state = `cm-conflict-${lookOf(chunk)}`
  return chunk.index === current ? `${state} cm-conflict-current` : state
}

const middleDecorations = EditorView.decorations.compute(
  ['doc', middleChunks, currentChunk, acceptHandler],
  (state) => {
    const current = state.field(currentChunk)
    const chunks = state.field(middleChunks)
    const onAccept = state.facet(acceptHandler)
    const lines = lineClasses(
      state.doc,
      chunks.map((chunk) => ({
        from: chunk.from,
        to: chunk.to,
        className: chunkClass(chunk, current)
      }))
    )
    const empty = chunks
      .filter((chunk) => chunk.from === chunk.to)
      .map((chunk) =>
        Decoration.widget({
          widget: new EmptyChunkWidget(chunk.index, chunk.resolved, lookOf(chunk), onAccept),
          block: true,
          side: -1
        }).range(chunk.from)
      )
    const accepts = onAccept
      ? chunks
          .filter((chunk) => chunk.from < chunk.to && lookOf(chunk) === 'proposed')
          .map((chunk) =>
            Decoration.widget({ widget: new AcceptWidget(chunk.index, onAccept), side: 1 }).range(
              state.doc.lineAt(chunk.from).to
            )
          )
      : []
    const add = [...empty, ...accepts]
    return add.length ? lines.update({ add, sort: true }) : lines
  }
)

// A side block is blue while that side is open and loses its color once decided, as used.
const sideDecorations = EditorView.decorations.compute(
  ['doc', sideChunks, sideDecisions, currentChunk],
  (state) => {
    const current = state.field(currentChunk)
    const decisions = state.field(sideDecisions)
    const doc = state.doc
    const chunks = state.field(sideChunks).filter(({ range }) => range.start <= doc.lines)
    const lines = lineClasses(
      doc,
      chunks.flatMap(({ index, range }) => {
        if (range.end < range.start) return []
        const last = doc.line(Math.min(range.end, doc.lines))
        return [
          {
            from: doc.line(range.start).from,
            to: Math.min(last.to + 1, doc.length),
            className: [
              (decisions[index] ?? 'open') === 'open' ? 'cm-conflict-side' : 'cm-conflict-used',
              index === current ? 'cm-conflict-current' : ''
            ]
              .filter(Boolean)
              .join(' ')
          }
        ]
      })
    )
    return lines
  }
)

const theme = EditorView.theme({
  '&': {
    height: '100%',
    fontSize: 'var(--text-xs)',
    backgroundColor: 'var(--color-surface-0)',
    color: 'var(--color-fg-primary)'
  },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.55' },
  // Chromium paints the corner where both scrollbars meet white unless told otherwise.
  '.cm-scroller::-webkit-scrollbar-corner': { background: 'transparent' },
  '.cm-gutters': {
    backgroundColor: 'var(--color-surface-0)',
    color: 'var(--color-fg-disabled)',
    border: 'none'
  },
  // The base theme draws a light border between the text and a gutter after it.
  '.cm-gutters.cm-gutters-after': { border: 'none !important' },
  '&.cm-focused': { outline: 'none' },
  '.cm-cursor': { borderLeftColor: 'var(--color-amber)' },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
    backgroundColor: 'var(--color-amber-muted) !important'
  },
  // Faint tints so syntax colors can come later, never amber, which marks comments: the middle is
  // red until something is picked and jade after, an open side is sapphire.
  '.cm-conflict-open': { backgroundColor: 'var(--color-error-muted)' },
  '.cm-conflict-picked': { backgroundColor: 'var(--color-jade-muted)' },
  // Violet is the model's: a proposal waiting for the human, and its edits outside the conflicts.
  '.cm-conflict-proposed': { backgroundColor: 'var(--color-violet-muted)' },
  '.cm-model-edit': { boxShadow: 'inset 3px 0 0 var(--color-violet-dim)' },
  '.cm-conflict-side': { backgroundColor: 'var(--color-sapphire-muted)' },
  '.cm-conflict-current': { boxShadow: 'inset 2px 0 0 var(--color-jade)' },
  '.cm-conflict-side.cm-conflict-current, .cm-conflict-used.cm-conflict-current': {
    boxShadow: 'inset 2px 0 0 var(--color-sapphire)'
  },
  '.cm-conflict-empty': {
    padding: '0 var(--space-2)',
    fontFamily: 'var(--font-body)',
    fontStyle: 'italic',
    color: 'var(--color-fg-muted)'
  },
  '.cm-conflict-decide .cm-gutterElement': { display: 'flex', alignItems: 'center' },
  '.cm-decide': {
    display: 'flex',
    alignItems: 'center',
    gap: '2px',
    padding: '0 2px'
  },
  '.cm-decide-button': {
    display: 'flex',
    padding: '1px',
    border: 'none',
    borderRadius: 'var(--radius-sm)',
    background: 'none',
    cursor: 'pointer'
  },
  // Both controls are drawn in full until a side is decided, then the one not chosen dims.
  '.cm-decide-button.take': { color: 'var(--color-jade)' },
  '.cm-decide-button.reject': { color: 'var(--color-error)' },
  '.cm-decide-button.on': { backgroundColor: 'var(--color-surface-2)' },
  '.cm-decide-button.off': { color: 'var(--color-fg-disabled)' },
  '.cm-decide-button.take.off:hover': { color: 'var(--color-jade)' },
  '.cm-decide-button.reject.off:hover': { color: 'var(--color-error)' },
  '.cm-lineNumbers-after .cm-gutterElement': { textAlign: 'left', padding: '0 5px 0 3px' },
  '.cm-conflict-hidden': {
    padding: '0 var(--space-2)',
    fontFamily: 'var(--font-body)',
    fontStyle: 'italic',
    color: 'var(--color-fg-muted)',
    backgroundColor: 'var(--color-surface-1)'
  },
  '.cm-conflict-empty.open': { backgroundColor: 'var(--color-error-muted)' },
  '.cm-conflict-empty.picked': { backgroundColor: 'var(--color-jade-muted)' },
  '.cm-conflict-empty.proposed': { backgroundColor: 'var(--color-violet-muted)' },
  // The go action for one block, sticky to the right edge so on a line wider than the pane it stays
  // in view. It then sits over the line's text, so it is solid: a see-through one lets the text
  // bleed into it.
  '.cm-accept': {
    position: 'sticky',
    right: '8px',
    marginLeft: '2ch',
    padding: '0 6px',
    background: 'var(--color-jade)',
    border: 'none',
    borderRadius: 'var(--radius-sm)',
    color: 'var(--color-black)',
    fontFamily: 'var(--font-body)',
    fontSize: 'var(--text-xs)',
    fontStyle: 'normal',
    fontWeight: 'var(--weight-medium)',
    cursor: 'pointer'
  },
  '.cm-accept:hover': { backgroundColor: 'var(--color-jade-dim)' }
})

function base(extra: Extension[]): Extension[] {
  return [drawSelection(), currentChunk, theme, ...extra]
}

type OnMiddleUpdate = (state: EditorState, docChanged: boolean) => void

function middleExtensions(onUpdate: OnMiddleUpdate, extra: Extension[]): Extension[] {
  return base([
    lineNumbers(),
    middleChunks,
    modelEdits,
    undoChunks,
    middleDecorations,
    modelEditDecorations,
    history(),
    keymap.of([...defaultKeymap, ...historyKeymap]),
    EditorView.updateListener.of((update) => {
      if (update.docChanged || update.selectionSet || update.transactions.length) {
        onUpdate(update.state, update.docChanged)
      }
    }),
    extra
  ])
}

export function middleState(
  doc: string,
  chunks: MiddleChunk[],
  onUpdate: OnMiddleUpdate,
  extra: Extension[],
  edits: GitLineRange[] = []
): EditorState {
  const state = EditorState.create({ doc })
  const ranges = edits
    .filter((range) => range.start >= 1 && range.end >= range.start && range.end <= state.doc.lines)
    .map((range) => ({
      from: state.doc.line(range.start).from,
      // Past the newline, so an edited empty line still has a range to mark.
      to: Math.min(state.doc.line(range.end).to + 1, state.doc.length)
    }))
  return EditorState.create({
    doc,
    extensions: [
      middleExtensions(onUpdate, extra),
      middleChunks.init(() => chunks),
      modelEdits.init(() => ranges)
    ]
  })
}

// Undo history is left out: its saved form drops the decision effects, so an undo after a restore
// would change the text without its decisions.
const savedFields = { chunks: middleChunks, edits: modelEdits }

// The middle as plain data, with its decisions, so it outlives its view.
export function snapshotMiddle(state: EditorState): unknown {
  return state.toJSON(savedFields)
}

// Rebuilds a saved middle with fresh extensions, since the saved one's callbacks belonged to a
// view that is gone.
export function restoreMiddle(
  json: unknown,
  onUpdate: OnMiddleUpdate,
  extra: Extension[]
): EditorState {
  return EditorState.fromJSON(json, { extensions: middleExtensions(onUpdate, extra) }, savedFields)
}

export function sideState(
  doc: string,
  chunks: SideChunk[],
  side: ConflictSide,
  branch: string,
  onDecide: OnDecide,
  extra: Extension[]
): EditorState {
  return EditorState.create({
    doc,
    extensions: base([
      sideChunks.init(() => chunks),
      sideDecisions,
      sideDecorations,
      // Read only but still editable as a view, so lines can be selected to comment on them.
      EditorState.readOnly.of(true),
      // Gutters on one side line up in extension order, so the decision gutter comes nearest the
      // middle and the extras' gutters, such as the comment one, end up on the outer edge.
      side === 'left'
        ? [extra, lineNumbersAfter, decisionGutter(side, branch, onDecide)]
        : [decisionGutter(side, branch, onDecide), lineNumbers(), extra]
    ])
  })
}

export function showDecisions(view: EditorView, decisions: Record<number, SideDecision>): void {
  view.dispatch({ effects: setDecisions.of(decisions) })
}

// Rewrites conflicts in the middle after a decision, as one undoable step. Each update carries the
// chunk's new decisions and its new text; positions are worked out here.
export function chunkReplacements(
  state: EditorState,
  updates: { chunk: MiddleChunk; text: string }[]
): TransactionSpec | null {
  const current = state.field(middleChunks)
  const sorted = updates
    .map((update) => ({ ...update, old: current.find((c) => c.index === update.chunk.index) }))
    .filter((update): update is typeof update & { old: MiddleChunk } => update.old !== undefined)
    .sort((a, b) => a.old.from - b.old.from)
  if (sorted.length === 0) return null
  let shift = 0
  const effects = sorted.map(({ chunk, text, old }) => {
    const from = old.from + shift
    shift += text.length - (old.to - old.from)
    return setChunk.of({ ...chunk, from, to: from + text.length })
  })
  const first = sorted[0].old.from
  return {
    changes: sorted.map(({ text, old }) => ({ from: old.from, to: old.to, insert: text })),
    effects:
      sorted.length === 1
        ? [...effects, EditorView.scrollIntoView(first, { y: 'center' })]
        : effects,
    selection: sorted.length === 1 ? { anchor: first } : undefined
  }
}

export function showCurrent(view: EditorView, index: number | null): void {
  if (view.state.field(currentChunk) !== index) view.dispatch({ effects: setCurrent.of(index) })
}

// The conflict the cursor sits in, counting the position right after an empty one.
export function chunkAtCursor(state: EditorState): number | null {
  const head = state.selection.main.head
  const chunk = state
    .field(middleChunks)
    .find((c) => (c.from === c.to ? head === c.from : head >= c.from && head < c.to))
  return chunk ? chunk.index : null
}

// A middle chunk's lines, 1-based and inclusive, with an empty chunk as end = start - 1.
export function chunkLines(doc: Text, chunk: MiddleChunk): GitLineRange {
  const start = doc.lineAt(chunk.from).number
  return { start, end: chunk.from === chunk.to ? start - 1 : doc.lineAt(chunk.to - 1).number }
}

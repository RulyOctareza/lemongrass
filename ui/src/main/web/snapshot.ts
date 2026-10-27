// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Turns a CDP Accessibility.getFullAXTree result into a tree of the lines the model reads, and gives the
// actionable elements refs so a later command can name one. Pure: no CDP, so it is testable on its own.
// Printing the tree, in full or as changes, is in print.ts.
import { createHash } from 'node:crypto'
import { WebError } from './types'

export interface AXValue {
  value?: unknown
}

export interface AXNode {
  nodeId: string
  ignored?: boolean
  ignoredReasons?: { name: string; value?: AXValue }[]
  role?: AXValue
  name?: AXValue
  value?: AXValue
  properties?: { name: string; value: AXValue }[]
  childIds?: string[]
  parentId?: string
  backendDOMNodeId?: number
}

export interface SnapshotRef {
  ref: string
  backendNodeId: number
  role: string
  name: string
  // The role and name the model last read for this ref. A node can be reused for other content, as when a
  // select refills its options, so an action checks the node still shows what the model chose.
  shown?: { role: string; name: string }
}

const describe = (role: string, name: string): string => (name ? `${role} "${name}"` : role)

export interface TreeOptions {
  // Backend node ids of password inputs: their value is never printed.
  masked: Set<number>
  // Names worked out from the page for actionable elements that have none, by backend node id.
  derived?: Map<number, DerivedName>
  // Backend node ids of elements the page marks clickable with a pointer cursor but gives no role.
  clickable?: Set<number>
}

// A name taken from the element's own markup, or when it has none a tag and stable attributes to tell it apart.
export interface DerivedName {
  name?: string
  hint?: string
}

// One printed line and the lines under it. `key` names the node across snapshots of one document, so print.ts
// can tell what the model has already seen; `hash` covers the line and everything under it.
export interface Line {
  key: string
  // The role and name part, before any ref: a fold line puts a ref given at print time after it.
  label: string
  // The ref, properties and value part.
  rest: string
  role: string
  name: string
  backendNodeId?: number
  ref?: string
  modal: boolean
  children: Line[]
  // Refs on this line and under it.
  refs: number
  hash: string
}

export const ACTIONABLE = new Set([
  'button',
  'link',
  'textbox',
  'searchbox',
  'combobox',
  'checkbox',
  'radio',
  'switch',
  'menuitem',
  'menuitemcheckbox',
  'menuitemradio',
  'tab',
  'option',
  'slider',
  'spinbutton',
  'treeitem'
])

// Roles that only group other nodes: with no name of their own they add nothing to the text.
const TRANSPARENT = new Set([
  'generic',
  'none',
  'presentation',
  'GenericContainer',
  'InlineTextBox',
  'LineBreak',
  'RootWebArea',
  'WebArea',
  'LabelText'
])

// A bullet or number drawn by the list's style says nothing a model can act on.
const DROPPED = new Set(['ListMarker'])

const TEXT_INPUTS = new Set(['textbox', 'searchbox', 'combobox'])

const MAX_TEXT = 200
const SHOWN_PROPERTIES = [
  'disabled',
  'checked',
  'expanded',
  'selected',
  'required',
  'invalid',
  'level'
]

export function axText(value: AXValue | undefined): string {
  const raw = value?.value
  return typeof raw === 'string' ? raw : typeof raw === 'number' ? String(raw) : ''
}

function clip(text: string): string {
  const flat = text.replace(/\s+/g, ' ').trim()
  return flat.length > MAX_TEXT ? flat.slice(0, MAX_TEXT) + '...' : flat
}

function propertyTags(node: AXNode): string {
  const tags: string[] = []
  for (const prop of node.properties ?? []) {
    if (!SHOWN_PROPERTIES.includes(prop.name)) continue
    const value = prop.value?.value
    if (value === false || value === 'false' || value === undefined) continue
    tags.push(value === true || value === 'true' ? prop.name : `${prop.name}=${String(value)}`)
  }
  return tags.length ? ' [' + tags.join(', ') + ']' : ''
}

function isModal(node: AXNode): boolean {
  return (node.properties ?? []).some((p) => p.name === 'modal' && p.value?.value === true)
}

// Most ignored nodes are plain wrappers; these reasons mean the page itself hides the node from people, as a
// closed drawer kept mounted with aria-hidden does.
const HIDDEN_REASONS = new Set([
  'ariaHiddenElement',
  'ariaHiddenSubtree',
  'inertElement',
  'inertSubtree'
])

export function hiddenByPage(node: AXNode): boolean {
  return (node.ignoredReasons ?? []).some((reason) => HIDDEN_REASONS.has(reason.name))
}

// The text an element shows, from the text nodes inside it, for a clickable element that has no name of its own.
// Text inside an actionable element is left out: that element prints its own line, and a wrapper around a select
// would otherwise be named after every option and change name each time the list refills.
export function textOf(node: AXNode, byId: Map<string, AXNode>): string {
  const parts: string[] = []
  const visit = (n: AXNode): void => {
    if (n !== node && ACTIONABLE.has(axText(n.role))) return
    if (axText(n.role) === 'StaticText') parts.push(axText(n.name))
    for (const id of n.childIds ?? []) {
      const child = byId.get(id)
      if (child) visit(child)
    }
  }
  visit(node)
  return clip(parts.join(' '))
}

// A ref given for a node that no longer exists.
export class RefGoneError extends WebError {}

// Refs bound to DOM nodes. A node keeps its ref for as long as its document lives, so a model can chain
// actions on refs it read earlier; only nodes seen for the first time take new numbers. Numbering never
// restarts, so a ref from a page the browser navigated away from is told apart from one never given.
export class RefBook {
  private byNode = new Map<number, SnapshotRef>()
  private issued = new Map<string, SnapshotRef>()
  private live = new Map<string, SnapshotRef>()
  private next: number
  private documentFirst: number
  private documentId: number | undefined

  constructor(first = 1) {
    this.next = first
    this.documentFirst = first
  }

  get nextRef(): number {
    return this.next
  }

  // Returns true when the document changed, which also drops every binding: its nodes are all new.
  enterDocument(id: number | undefined): boolean {
    if (id === this.documentId) return false
    this.documentId = id
    this.byNode.clear()
    this.issued.clear()
    this.live.clear()
    this.documentFirst = this.next
    return true
  }

  // Starts a new tree: only refs met while building it count as in the page.
  beginTree(): void {
    this.live = new Map()
  }

  refFor(backendNodeId: number, role: string, name: string): string {
    let found = this.byNode.get(backendNodeId)
    if (!found) {
      found = { ref: `e${this.next++}`, backendNodeId, role, name }
      this.byNode.set(backendNodeId, found)
      this.issued.set(found.ref, found)
    } else {
      found.role = role
      found.name = name
    }
    this.live.set(found.ref, found)
    return found.ref
  }

  // A node that once had a ref, such as a folded container, is still in the page under that ref.
  keep(backendNodeId: number): void {
    const found = this.byNode.get(backendNodeId)
    if (found) this.live.set(found.ref, found)
  }

  markShown(ref: string): void {
    const found = this.issued.get(ref)
    if (found) found.shown = { role: found.role, name: found.name }
  }

  // Refuses a ref whose node now shows other content than the model last read for it. The refusal prints the
  // new content, so it counts as shown.
  checkShown(ref: string): void {
    const found = this.lookup(ref)
    const shown = found.shown
    if (!shown || (shown.role === found.role && shown.name === found.name)) return
    this.markShown(ref)
    throw new WebError(
      `${ref} now holds ${describe(found.role, found.name)}, not ${describe(shown.role, shown.name)} as last printed; the page reused that element, use the ref from the latest output`
    )
  }

  liveRefs(): SnapshotRef[] {
    return [...this.live.values()]
  }

  lookup(ref: string): SnapshotRef {
    const match = /^e(\d+)$/.exec(ref)
    if (!match)
      throw new WebError(`${ref} is not a ref; refs look like e12 and come from a snapshot`)
    const live = this.live.get(ref)
    if (live) return live
    const gone = this.issued.get(ref)
    if (gone) {
      const what = gone.shown
        ? describe(gone.shown.role, gone.shown.name)
        : describe(gone.role, gone.name)
      throw new RefGoneError(
        `${ref} (${what}) is no longer in the page; run snapshot to see what is there now`
      )
    }
    if (Number(match[1]) < this.documentFirst)
      throw new RefGoneError(
        `${ref} was on an earlier page, the browser has navigated since; run snapshot`
      )
    throw new WebError(`${ref} was never given on this page; run snapshot`)
  }
}

function hashOf(text: string, children: Line[]): string {
  const h = createHash('sha1').update(text)
  for (const child of children) h.update('\n' + child.hash)
  return h.digest('base64')
}

export function makeLine(fields: Omit<Line, 'refs' | 'hash'>): Line {
  const refs = (fields.ref ? 1 : 0) + fields.children.reduce((sum, c) => sum + c.refs, 0)
  return { ...fields, refs, hash: hashOf(fields.label + fields.rest, fields.children) }
}

export function lineText(line: Line): string {
  return line.label + line.rest
}

// The page as a tree of lines under a root keyed "page". Static text repeated by its parent or by a text input
// is dropped and generic wrappers are flattened.
export function buildTree(nodes: AXNode[], options: TreeOptions, book: RefBook): Line {
  const byId = new Map(nodes.map((node) => [node.nodeId, node]))
  const root = nodes.find((node) => node.parentId === undefined) ?? nodes[0]
  book.beginTree()

  const keyOf = (node: AXNode, parentKey: string, index: number): string =>
    node.backendDOMNodeId !== undefined ? `b${node.backendDOMNodeId}` : `${parentKey}/${index}`

  const walk = (node: AXNode, key: string, parentName: string, parentRole: string): Line[] => {
    const role = axText(node.role)
    const name = clip(axText(node.name))
    const children = (node.childIds ?? []).map((id) => byId.get(id)).filter((n) => n !== undefined)
    const descend = (childParentName: string, childParentRole: string): Line[] =>
      children.flatMap((child, i) =>
        walk(child, keyOf(child, key, i), childParentName, childParentRole)
      )
    const backendNodeId = node.backendDOMNodeId
    if (backendNodeId !== undefined) book.keep(backendNodeId)

    const asClickable =
      backendNodeId !== undefined &&
      options.clickable?.has(backendNodeId) === true &&
      !ACTIONABLE.has(role) &&
      role !== 'StaticText' &&
      role !== 'RootWebArea'
    if (asClickable) {
      const text = name || textOf(node, byId)
      const derived = text === '' ? options.derived?.get(backendNodeId) : undefined
      const shown = derived?.name ? clip(derived.name) : text
      let label = 'clickable'
      if (shown) label += ` "${shown}"`
      else if (derived?.hint) label += ` (unnamed: ${derived.hint})`
      const ref = book.refFor(backendNodeId, 'clickable', shown)
      return [
        makeLine({
          key,
          label,
          rest: ` [ref=${ref}]`,
          role: 'clickable',
          name: shown,
          backendNodeId,
          ref,
          modal: false,
          children: descend(shown, 'clickable')
        })
      ]
    }
    if (DROPPED.has(role)) return []
    if (node.ignored || TRANSPARENT.has(role) || role === '') return descend(parentName, parentRole)
    if (role === 'StaticText') {
      // A link or heading already carries its text as its name, and a text input its value.
      if (name === '' || TEXT_INPUTS.has(parentRole) || parentName.includes(name)) return []
      const label = `text "${name}"`
      return [
        makeLine({ key, label, rest: '', role, name, backendNodeId, modal: false, children: [] })
      ]
    }

    const actionable = ACTIONABLE.has(role) && backendNodeId !== undefined
    const derived = name === '' && actionable ? options.derived?.get(backendNodeId) : undefined
    const shown = derived?.name ? clip(derived.name) : name
    let label = shown === '' ? role : `${role} "${shown}"`
    if (shown === '' && derived?.hint) label += ` (unnamed: ${derived.hint})`
    let rest = ''
    let ref: string | undefined
    if (actionable) {
      ref = book.refFor(backendNodeId, role, shown)
      rest += ` [ref=${ref}]`
    }
    rest += propertyTags(node)
    const value = axText(node.value)
    if (value !== '') {
      const masked = backendNodeId !== undefined && options.masked.has(backendNodeId)
      rest += masked ? ' value="[masked]"' : ` value="${clip(value)}"`
    }
    return [
      makeLine({
        key,
        label,
        rest,
        role,
        name: shown,
        backendNodeId,
        ref,
        modal: (role === 'dialog' || role === 'alertdialog') && isModal(node),
        children: descend(shown, role)
      })
    ]
  }

  const top = root ? walk(root, 'page', '', '') : []
  // The root itself is transparent, so walking it gives the page's top level lines.
  return makeLine({
    key: 'page',
    label: 'page',
    rest: '',
    role: 'RootWebArea',
    name: '',
    modal: false,
    children: top
  })
}

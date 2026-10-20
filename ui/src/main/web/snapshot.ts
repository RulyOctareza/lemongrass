// Turns a CDP Accessibility.getFullAXTree result into the indented text the model reads, and numbers
// the actionable elements so a later command can name one. Pure: no CDP, so it is testable on its own.

export interface AXValue {
  value?: unknown
}

export interface AXNode {
  nodeId: string
  ignored?: boolean
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
}

export interface SnapshotOptions {
  url: string
  title: string
  // Numbering continues across snapshots so a ref names one snapshot only.
  firstRef: number
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

export interface SnapshotResult {
  text: string
  refs: SnapshotRef[]
  nextRef: number
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

const TEXT_INPUTS = new Set(['textbox', 'searchbox', 'combobox'])

const MAX_LINES = 600
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

// The text an element shows, from the text nodes inside it, for a clickable element that has no name of its own.
function textOf(node: AXNode, byId: Map<string, AXNode>): string {
  const parts: string[] = []
  const visit = (n: AXNode): void => {
    if (axText(n.role) === 'StaticText') parts.push(axText(n.name))
    for (const id of n.childIds ?? []) {
      const child = byId.get(id)
      if (child) visit(child)
    }
  }
  visit(node)
  return clip(parts.join(' '))
}

export function formatSnapshot(nodes: AXNode[], options: SnapshotOptions): SnapshotResult {
  const byId = new Map(nodes.map((node) => [node.nodeId, node]))
  const root = nodes.find((node) => node.parentId === undefined) ?? nodes[0]
  const lines: string[] = []
  const refs: SnapshotRef[] = []
  let next = options.firstRef
  let truncated = 0

  const emit = (depth: number, line: string): void => {
    if (lines.length >= MAX_LINES) {
      truncated++
      return
    }
    lines.push('  '.repeat(depth) + line)
  }

  const walk = (node: AXNode, depth: number, parentName: string, parentRole = ''): void => {
    const role = axText(node.role)
    const name = clip(axText(node.name))
    const children = (node.childIds ?? []).map((id) => byId.get(id)).filter((n) => n !== undefined)
    const descend = (childDepth: number, childParentName: string): void => {
      for (const child of children) walk(child, childDepth, childParentName, parentRole)
    }

    const asClickable =
      node.backendDOMNodeId !== undefined &&
      options.clickable?.has(node.backendDOMNodeId) === true &&
      !ACTIONABLE.has(role) &&
      role !== 'StaticText' &&
      role !== 'RootWebArea'
    if (asClickable && node.backendDOMNodeId !== undefined) {
      const text = name || textOf(node, byId)
      const ref = `e${next++}`
      refs.push({ ref, backendNodeId: node.backendDOMNodeId, role: 'clickable', name: text })
      emit(depth, `clickable${text ? ` "${text}"` : ''} [ref=${ref}]`)
      for (const child of children) walk(child, depth + 1, text, 'clickable')
      return
    }
    if (node.ignored || TRANSPARENT.has(role) || role === '') {
      descend(depth, parentName)
      return
    }
    if (role === 'StaticText') {
      // A link or heading already carries its text as its name, and a text input its value.
      if (name !== '' && !TEXT_INPUTS.has(parentRole) && !parentName.includes(name))
        emit(depth, `text "${name}"`)
      return
    }

    const backendNodeId = node.backendDOMNodeId
    const actionable = ACTIONABLE.has(role) && backendNodeId !== undefined
    const derived = name === '' && actionable ? options.derived?.get(backendNodeId) : undefined
    const shown = derived?.name ? clip(derived.name) : name
    let line = shown === '' ? role : `${role} "${shown}"`
    if (shown === '' && derived?.hint) line += ` (unnamed: ${derived.hint})`
    if (actionable) {
      const ref = `e${next++}`
      refs.push({ ref, backendNodeId, role, name: shown })
      line += ` [ref=${ref}]`
    }
    line += propertyTags(node)
    const value = axText(node.value)
    if (value !== '') {
      const masked = backendNodeId !== undefined && options.masked.has(backendNodeId)
      line += masked ? ' value="[masked]"' : ` value="${clip(value)}"`
    }
    emit(depth, line)
    for (const child of children) walk(child, depth + 1, shown, role)
  }

  if (root) walk(root, 0, '')
  if (truncated > 0) lines.push(`... ${truncated} more lines not shown`)
  const header = [`url: ${options.url}`, `title: ${clip(options.title)}`, 'page:']
  return { text: [...header, ...lines].join('\n'), refs, nextRef: next }
}

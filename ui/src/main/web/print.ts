// Prints a tree from snapshot.ts as the indented text the model reads: the whole page, one subtree, or only
// what changed since the model last saw it. Pure, like snapshot.ts.
import { lineText, type Line, type RefBook } from './snapshot'

const MAX_LINES = 600
// A long list is mostly rows the model does not need to read to act on one; the rest are a count away.
const MAX_ITEMS = 20
const MAX_REMOVED = 10
const MAX_FOLDS = 3
const LISTS = new Set([
  'list',
  'listbox',
  'menu',
  'menubar',
  'MenuListPopup',
  'tree',
  'treegrid',
  'grid',
  'table',
  'rowgroup'
])
// Lists a model narrows by typing into the field that opened them, such as the rows of a searchable select.
const FILTERABLE = new Set(['list', 'listbox', 'menu', 'MenuListPopup'])

interface Seen {
  hash: string
  text: string
  refs: number
  parent: string
}

// What the model has been shown of one document, by line key. A line counts as shown once it was printed,
// folded as unchanged or counted in a folded list; the page behind a modal dialog is not, so its changes
// still print once the dialog closes.
export class Baseline {
  private seen = new Map<string, Seen>()

  clear(): void {
    this.seen.clear()
  }

  same(line: Line): boolean {
    return this.seen.get(line.key)?.hash === line.hash
  }

  record(line: Line, parent: string, deep: boolean): void {
    this.seen.set(line.key, { hash: line.hash, text: lineText(line), refs: line.refs, parent })
    if (deep) for (const child of line.children) this.record(child, line.key, true)
  }

  parentOf(key: string): string {
    return this.seen.get(key)?.parent ?? ''
  }

  // The topmost lines the model saw that are no longer in the tree. They are forgotten as they are reported.
  removed(tree: Line): Seen[] {
    const present = new Set<string>()
    const visit = (line: Line): void => {
      present.add(line.key)
      line.children.forEach(visit)
    }
    visit(tree)
    const gone = [...this.seen.keys()].filter((key) => !present.has(key))
    const goneSet = new Set(gone)
    const top = gone.map((key) => this.seen.get(key) as Seen).filter((s) => !goneSet.has(s.parent))
    for (const key of gone) this.seen.delete(key)
    return top
  }
}

export interface Header {
  url: string
  title: string
}

const plural = (n: number, word: string): string => `${n} ${word}${n === 1 ? '' : 's'}`

class Printer {
  readonly lines: string[] = []
  private truncated = 0

  constructor(
    private readonly book: RefBook,
    private readonly baseline: Baseline,
    // Unchanged lines fold instead of printing.
    private readonly diff: boolean,
    // Long lists print every item.
    private readonly unfold: boolean
  ) {}

  emit(depth: number, text: string): void {
    if (this.lines.length >= MAX_LINES) {
      this.truncated++
      return
    }
    this.lines.push('  '.repeat(depth) + text)
  }

  finish(): string[] {
    if (this.truncated > 0) this.lines.push(`... ${this.truncated} more lines not shown`)
    return this.lines
  }

  // A ref for a line that has none of its own, so snapshot --within can open what a fold line hides.
  private refOf(line: Line): string | undefined {
    if (line.ref) return line.ref
    if (line.backendNodeId === undefined) return undefined
    return this.book.refFor(line.backendNodeId, line.role, line.name)
  }

  fold(line: Line): string {
    if (line.ref) this.book.markShown(line.ref)
    const given = line.ref ? undefined : this.refOf(line)
    const ref = given ? ` [ref=${given}]` : ''
    return `${line.label}${ref}${line.rest}: unchanged, ${plural(line.refs, 'ref')}`
  }

  line(line: Line, depth: number, parent: string): void {
    this.emit(depth, lineText(line))
    if (line.ref) this.book.markShown(line.ref)
    this.baseline.record(line, parent, false)
    this.children(line, depth + 1)
  }

  children(line: Line, depth: number): void {
    const items = line.children
    const listed = !this.unfold && LISTS.has(line.role) && items.length > MAX_ITEMS
    let run: Line[] = []
    const count = (lines: Line[]): void => {
      if (lines.length === 0) return
      const refs = lines.reduce((sum, l) => sum + l.refs, 0)
      this.emit(depth, `(${lines.length} unchanged${refs ? `, ${plural(refs, 'ref')}` : ''})`)
    }
    // A few unchanged containers, such as a sidebar beside a form, each keep a fold line with a ref, so
    // snapshot --within can open one; plain lines between them and long runs of rows are only counted.
    const flush = (): void => {
      const containers = run.filter((l) => l.children.length > 0).length
      if (containers === 0 || containers > MAX_FOLDS) count(run)
      else {
        let leaves: Line[] = []
        for (const l of run) {
          if (l.children.length === 0) {
            leaves.push(l)
            continue
          }
          count(leaves)
          leaves = []
          this.emit(depth, this.fold(l))
        }
        count(leaves)
      }
      run = []
    }
    for (const child of listed ? items.slice(0, MAX_ITEMS) : items) {
      if (this.diff && this.baseline.same(child)) {
        run.push(child)
        continue
      }
      flush()
      this.line(child, depth, line.key)
    }
    if (listed) {
      const hidden = items.slice(MAX_ITEMS)
      const changed = this.diff
        ? hidden.filter((c) => !this.baseline.same(c)).length
        : hidden.length
      for (const child of hidden) this.baseline.record(child, line.key, true)
      if (changed === 0) run.push(...hidden)
      else {
        flush()
        const note = changed < hidden.length ? ` (${changed} changed)` : ''
        const ref = this.refOf(line)
        const ways = [
          FILTERABLE.has(line.role) ? 'type to filter' : '',
          ref ? `snapshot --within ${ref}` : ''
        ].filter((w) => w !== '')
        this.emit(
          depth,
          `... ${plural(hidden.length, 'more item')}${note}${ways.length ? ', ' + ways.join(' or ') : ''}`
        )
      }
    }
    flush()
  }
}

function headerLines(header: Header): string[] {
  const title = header.title.replace(/\s+/g, ' ').trim()
  return [
    `url: ${header.url}`,
    `title: ${title.length > 200 ? title.slice(0, 200) + '...' : title}`
  ]
}

// The whole page. It becomes what later action results are compared against.
export function printFull(tree: Line, header: Header, book: RefBook, baseline: Baseline): string {
  baseline.clear()
  const printer = new Printer(book, baseline, false, false)
  printer.children(tree, 0)
  baseline.record(tree, '', false)
  return [...headerLines(header), 'page:', ...printer.finish()].join('\n')
}

export function findLine(tree: Line, key: string): Line | undefined {
  if (tree.key === key) return tree
  for (const child of tree.children) {
    const found = findLine(child, key)
    if (found) return found
  }
  return undefined
}

// One subtree with every list item, for when the model needs what a fold line or an earlier result held.
export function printWithin(
  target: Line,
  ref: string,
  header: Header,
  book: RefBook,
  baseline: Baseline
): string {
  const printer = new Printer(book, baseline, false, true)
  printer.line(target, 0, baseline.parentOf(target.key))
  const what = target.name ? `${target.role} "${target.name}"` : target.role
  return [...headerLines(header), `within ${ref} (${what}):`, ...printer.finish()].join('\n')
}

interface ModalPick {
  dialog: Line
  parent: string
  // Lines after the dialog in page order, such as a dropdown the dialog opened at the end of body.
  after: { line: Line; parent: string }[]
}

function findModal(tree: Line): ModalPick | undefined {
  const path: Line[] = []
  const search = (line: Line): Line | undefined => {
    if (line.modal) return line
    path.push(line)
    for (const child of line.children) {
      const found = search(child)
      if (found) return found
    }
    path.pop()
    return undefined
  }
  const dialog = search(tree)
  if (!dialog) return undefined
  const after: ModalPick['after'] = []
  let at = dialog
  for (let i = path.length - 1; i >= 0; i--) {
    const parent = path[i]
    const index = parent.children.indexOf(at)
    for (const sibling of parent.children.slice(index + 1))
      after.push({ line: sibling, parent: parent.key })
    at = parent
  }
  return { dialog, parent: path[path.length - 1]?.key ?? 'page', after }
}

// Only what changed since the model last saw this document: an unchanged subtree folds to one line with its ref
// count, a changed line prints with its ancestors for context, and lines that left the page are listed at the end.
// An open modal dialog prints alone, since the page behind it cannot be used until it closes.
export function printChanges(
  tree: Line,
  header: Header,
  book: RefBook,
  baseline: Baseline
): string {
  const removed = baseline.removed(tree)
  const printer = new Printer(book, baseline, true, false)
  const lines = [...headerLines(header)]
  const modal = findModal(tree)
  if (modal) {
    lines.push('page: a modal dialog is open, the page behind it is not shown (snapshot prints it)')
    for (const { line, parent } of [{ line: modal.dialog, parent: modal.parent }, ...modal.after]) {
      if (baseline.same(line)) printer.emit(0, printer.fold(line))
      else printer.line(line, 0, parent)
    }
  } else if (baseline.same(tree)) {
    lines.push(`page: unchanged, ${plural(tree.refs, 'ref')}`)
  } else {
    lines.push('page:')
    printer.children(tree, 0)
    baseline.record(tree, '', false)
  }
  lines.push(...printer.finish())
  for (const gone of removed.slice(0, MAX_REMOVED))
    lines.push(`removed: ${gone.text}${gone.refs ? ` (${plural(gone.refs, 'ref')})` : ''}`)
  if (removed.length > MAX_REMOVED)
    lines.push(`... and ${removed.length - MAX_REMOVED} more removed`)
  return lines.join('\n')
}

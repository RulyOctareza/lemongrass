import assert from 'node:assert/strict'
import { test } from 'node:test'
import { needsDerivedName } from './names.ts'
import { Baseline, printFull } from './print.ts'
import { RefBook, RefGoneError, buildTree, type AXNode, type TreeOptions } from './snapshot.ts'

const v = (value: unknown): { value: unknown } => ({ value })

interface Formatted {
  text: string
  refs: { ref: string; backendNodeId: number; name: string }[]
  nextRef: number
}

function formatSnapshot(
  nodes: AXNode[],
  options: TreeOptions & { url: string; title: string; firstRef: number }
): Formatted {
  const book = new RefBook(options.firstRef)
  const tree = buildTree(nodes, options, book)
  const text = printFull(tree, { url: options.url, title: options.title }, book, new Baseline())
  return { text, refs: book.liveRefs(), nextRef: book.nextRef }
}

const tree: AXNode[] = [
  { nodeId: '1', role: v('RootWebArea'), childIds: ['2', '3', '4', '5', '9'] },
  {
    nodeId: '2',
    role: v('heading'),
    name: v('Sign in'),
    childIds: ['6'],
    parentId: '1',
    properties: [{ name: 'level', value: v(1) }]
  },
  { nodeId: '6', role: v('StaticText'), name: v('Sign in'), parentId: '2' },
  {
    nodeId: '3',
    role: v('textbox'),
    name: v('Email'),
    value: v('a@b.c'),
    backendDOMNodeId: 10,
    parentId: '1'
  },
  {
    nodeId: '4',
    role: v('textbox'),
    name: v('Password'),
    value: v('hunter2'),
    backendDOMNodeId: 11,
    parentId: '1'
  },
  { nodeId: '5', role: v('generic'), parentId: '1', childIds: ['7'] },
  {
    nodeId: '7',
    role: v('button'),
    name: v('Go'),
    backendDOMNodeId: 12,
    parentId: '5',
    properties: [{ name: 'disabled', value: v(true) }]
  },
  { nodeId: '9', role: v('StaticText'), name: v('Welcome back'), parentId: '1' }
]

test('numbers actionable elements from firstRef and masks password values', () => {
  const out = formatSnapshot(tree, {
    url: 'http://x/',
    title: 'T',
    firstRef: 5,
    masked: new Set([11])
  })
  assert.deepEqual(
    out.refs.map((r) => [r.ref, r.backendNodeId]),
    [
      ['e5', 10],
      ['e6', 11],
      ['e7', 12]
    ]
  )
  assert.equal(out.nextRef, 8)
  assert.ok(out.text.includes('textbox "Password" [ref=e6] value="[masked]"'))
  assert.ok(!out.text.includes('hunter2'))
  assert.ok(out.text.includes('textbox "Email" [ref=e5] value="a@b.c"'))
  assert.ok(out.text.includes('button "Go" [ref=e7] [disabled]'))
  assert.ok(out.text.includes('text "Welcome back"'))
})

test('drops static text repeated by its parent and flattens generic nodes', () => {
  const out = formatSnapshot(tree, { url: 'http://x/', title: 'T', firstRef: 1, masked: new Set() })
  assert.equal(out.text.split('Sign in').length - 1, 1)
  assert.ok(out.text.includes('heading "Sign in" [level=1]'))
  assert.ok(!out.text.includes('generic'))
})

const banner: AXNode[] = [
  { nodeId: '1', role: v('RootWebArea'), childIds: ['2', '3', '4', '5'] },
  { nodeId: '2', role: v('button'), backendDOMNodeId: 20, parentId: '1' },
  { nodeId: '3', role: v('button'), backendDOMNodeId: 21, parentId: '1' },
  { nodeId: '4', role: v('link'), name: v('Home'), backendDOMNodeId: 22, parentId: '1' },
  { nodeId: '5', role: v('button'), backendDOMNodeId: 23, parentId: '1' }
]

test('an unnamed button takes the name found in its markup, or a tag and stable hint', () => {
  const derived = new Map([
    [20, { name: 'Notifications' }],
    [21, { hint: 'button type=button icon=fa-bell' }]
  ])
  const out = formatSnapshot(banner, {
    url: 'http://x/',
    title: 'T',
    firstRef: 1,
    masked: new Set(),
    derived
  })
  assert.ok(out.text.includes('button "Notifications" [ref=e1]'))
  assert.ok(out.text.includes('button (unnamed: button type=button icon=fa-bell) [ref=e2]'))
  assert.ok(out.text.includes('link "Home" [ref=e3]'))
  assert.ok(out.text.includes('\nbutton [ref=e4]'))
  assert.equal(out.refs[0].name, 'Notifications')
})

test('only actionable nodes without a name ask for a derived name', () => {
  assert.deepEqual(
    banner.filter(needsDerivedName).map((n) => n.backendDOMNodeId),
    [20, 21, 23]
  )
  assert.equal(
    needsDerivedName({ nodeId: '9', ignored: true, role: v('button'), backendDOMNodeId: 1 }),
    false
  )
})

test('a node keeps its ref across snapshots and only new nodes take new numbers', () => {
  const book = new RefBook()
  buildTree(banner, { masked: new Set() }, book)
  assert.equal(book.lookup('e3').backendNodeId, 22)
  const grown: AXNode[] = [
    { nodeId: '1', role: v('RootWebArea'), childIds: ['6', '4'] },
    { nodeId: '6', role: v('button'), name: v('New'), backendDOMNodeId: 30, parentId: '1' },
    { nodeId: '4', role: v('link'), name: v('Home'), backendDOMNodeId: 22, parentId: '1' }
  ]
  const text = printFull(
    buildTree(grown, { masked: new Set() }, book),
    { url: '', title: '' },
    book,
    new Baseline()
  )
  assert.ok(text.includes('link "Home" [ref=e3]'))
  assert.ok(text.includes('button "New" [ref=e5]'))
  assert.throws(
    () => book.lookup('e1'),
    (err) => err instanceof RefGoneError && /no longer in the page/.test(err.message)
  )
  assert.throws(() => book.lookup('e99'), /never given/)
})

test('a new document drops every binding and refs before it say the page changed', () => {
  const book = new RefBook()
  book.enterDocument(1)
  buildTree(banner, { masked: new Set() }, book)
  assert.equal(book.enterDocument(1), false)
  assert.equal(book.enterDocument(2), true)
  buildTree(banner, { masked: new Set() }, book)
  assert.equal(book.lookup('e5').backendNodeId, 20)
  assert.throws(() => book.lookup('e2'), /earlier page/)
})

test('a ref whose node now shows other content than last printed is refused, once', () => {
  const book = new RefBook()
  const options = (name: string): AXNode[] => [
    { nodeId: '1', role: v('RootWebArea'), childIds: ['2'] },
    { nodeId: '2', role: v('option'), name: v(name), backendDOMNodeId: 40, parentId: '1' }
  ]
  const tree = buildTree(options('Alice'), { masked: new Set() }, book)
  printFull(tree, { url: '', title: '' }, book, new Baseline())
  buildTree(options('Dana'), { masked: new Set() }, book)
  assert.throws(
    () => book.checkShown('e1'),
    /e1 now holds option "Dana", not option "Alice" as last printed/
  )
  assert.doesNotThrow(() => book.checkShown('e1'))
})

test('a clickable wrapper takes no name from the actionable elements inside it', () => {
  const wrapper: AXNode[] = [
    { nodeId: '1', role: v('RootWebArea'), childIds: ['2'] },
    {
      nodeId: '2',
      role: v('generic'),
      backendDOMNodeId: 50,
      parentId: '1',
      childIds: ['3', '4', '6']
    },
    { nodeId: '3', role: v('StaticText'), name: v('Pick one'), parentId: '2' },
    {
      nodeId: '4',
      role: v('option'),
      name: v('Alice'),
      backendDOMNodeId: 51,
      parentId: '2',
      childIds: ['5']
    },
    { nodeId: '5', role: v('StaticText'), name: v('Alice'), parentId: '4' },
    { nodeId: '6', role: v('option'), name: v('Bruno'), backendDOMNodeId: 52, parentId: '2' }
  ]
  const out = formatSnapshot(wrapper, {
    url: '',
    title: '',
    firstRef: 1,
    masked: new Set(),
    clickable: new Set([50])
  })
  assert.ok(out.text.includes('clickable "Pick one" [ref=e1]'))
  assert.ok(out.text.includes('option "Alice" [ref=e2]'))
})

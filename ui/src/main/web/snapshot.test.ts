import assert from 'node:assert/strict'
import { test } from 'node:test'
import { needsDerivedName } from './names.ts'
import { formatSnapshot, type AXNode } from './snapshot.ts'

const v = (value: unknown): { value: unknown } => ({ value })

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

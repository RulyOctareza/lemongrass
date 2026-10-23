import assert from 'node:assert/strict'
import { test } from 'node:test'
import { Capture } from './capture.ts'

function newCapture(): Capture {
  return new Capture({ send: async () => ({}) as never }, () => ({
    tokenPlacement: null,
    tokenPath: ''
  }))
}

let seq = 0

function request(
  capture: Capture,
  method: string,
  url: string,
  status: number,
  extra: { postData?: string; headers?: Record<string, string> } = {}
): void {
  const requestId = `q${++seq}`
  capture.handle('Network.requestWillBeSent', {
    requestId,
    type: 'XHR',
    request: { url, method, headers: extra.headers ?? {}, postData: extra.postData }
  })
  capture.handle('Network.responseReceived', {
    requestId,
    response: { status, mimeType: 'application/json', headers: {} }
  })
  capture.handle('Network.loadingFinished', { requestId })
}

function consoleError(capture: Capture, text: string): void {
  capture.handle('Runtime.consoleAPICalled', { type: 'error', args: [{ value: text }] })
}

test('an action result prints only counts and the command that lists them', () => {
  const capture = newCapture()
  capture.action = 3
  request(capture, 'GET', 'http://h/api/unread', 200)
  request(capture, 'GET', 'http://h/api/unread', 200)
  request(capture, 'POST', 'http://h/api/save', 422, { postData: '{"name":""}' })
  consoleError(capture, 'save failed')
  const summary = capture.actionSummary(3)
  assert.equal(
    summary,
    'requests: 3 new (1 failed, 1 non-GET, 1 repeats), list them with network --since 3\n' +
      'console: 1 error, list them with console --errors'
  )
  assert.doesNotMatch(summary, /api\/save|name/)
  assert.equal(capture.actionSummary(4), 'requests: none')
})

test('network prints one line per request and folds identical ones with their count', () => {
  const capture = newCapture()
  capture.action = 1
  request(capture, 'GET', 'http://h/api/unread', 200)
  capture.action = 2
  request(capture, 'GET', 'http://h/api/unread', 200)
  request(capture, 'GET', 'http://h/api/unread', 500)
  request(capture, 'POST', 'http://h/api/save', 201, { postData: '{"a":1}' })
  const lines = capture.network({ failed: false }).split('\n')
  assert.match(lines[0], /^r\d+ \(actions 1-2\) GET http:\/\/h\/api\/unread 200 ×2$/)
  assert.match(lines[1], /^r\d+ \(action 2\) GET http:\/\/h\/api\/unread 500$/)
  assert.match(lines[2], /^r\d+ \(action 2\) POST http:\/\/h\/api\/save 201$/)
  assert.equal(lines[3], 'one request in full: network r<id>')
  assert.doesNotMatch(lines.join('\n'), /"a":1/)
})

test('network r<id> prints one request with redacted headers and bodies', () => {
  const capture = newCapture()
  request(capture, 'POST', 'http://h/api/save', 201, {
    postData: '{"a":1}',
    headers: { Authorization: 'Bearer abc.def', Accept: 'application/json' }
  })
  const id = Number(/^r(\d+)/.exec(capture.network({ failed: false }))?.[1])
  const text = capture.request(id) ?? ''
  assert.match(text, new RegExp(`^r${id} \\(action 0\\) POST http://h/api/save 201`))
  assert.match(text, /Authorization: \[redacted\]/)
  assert.match(text, /request body: \{"a":1\}/)
  assert.doesNotMatch(text, /abc\.def/)
  assert.equal(capture.request(9999), undefined)
})

test('console folds identical messages with their count', () => {
  const capture = newCapture()
  capture.action = 1
  consoleError(capture, 'boom')
  capture.action = 2
  consoleError(capture, 'boom')
  consoleError(capture, 'other')
  assert.equal(
    capture.consoleLog({ errors: true }),
    '[error] (actions 1-2) boom ×2\n[error] (action 2) other'
  )
})

test('human input is counted once, the engine own clicks are not, and its requests are marked', () => {
  const capture = newCapture()
  capture.startAction()
  capture.expectClick()
  capture.mouseDown()
  assert.equal(capture.takeHumanSummary(), '')
  request(capture, 'GET', 'http://h/api/list', 200)
  capture.mouseDown()
  capture.keyDown()
  capture.keyDown()
  request(capture, 'POST', 'http://h/api/save', 201)
  assert.equal(
    capture.takeHumanSummary(),
    'human input since your last call: 1 click, 2 keys, 1 request (network --since 1 marks them human)'
  )
  assert.equal(capture.takeHumanSummary(), '')
  const listing = capture.network({ failed: false })
  assert.match(listing, /\(action 1\) GET http:\/\/h\/api\/list 200/)
  assert.match(listing, /\(action 1\) \(human\) POST http:\/\/h\/api\/save 201/)
  capture.startAction()
  request(capture, 'GET', 'http://h/api/after', 200)
  assert.doesNotMatch(capture.network({ failed: false }), /\(human\) GET http:\/\/h\/api\/after/)
})

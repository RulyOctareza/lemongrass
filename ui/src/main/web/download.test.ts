import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { DownloadItem } from 'electron'
import { Capture } from './capture.ts'
import { Downloads } from './download.ts'

interface Fake {
  item: DownloadItem
  cancelled: () => boolean
  savePath: () => string
  finish: (state: string) => void
}

function fakeItem(): Fake {
  let cancelled = false
  let savePath = ''
  let done: (event: unknown, state: string) => void = () => undefined
  const item = {
    cancel: () => {
      cancelled = true
    },
    setSavePath: (path: string) => {
      savePath = path
    },
    getFilename: () => 'report.csv',
    getTotalBytes: () => 8,
    getReceivedBytes: () => 0,
    on: () => item,
    once: (_name: string, listener: typeof done) => {
      done = listener
      return item
    }
  }
  return {
    item: item as unknown as DownloadItem,
    cancelled: () => cancelled,
    savePath: () => savePath,
    finish: (state) => done(undefined, state)
  }
}

function newCapture(): Capture {
  return new Capture({ send: async () => ({}) as never }, () => ({
    tokenPlacement: null,
    tokenPath: ''
  }))
}

test('a download after a model action that did not ask for it is cancelled and named once', () => {
  const capture = newCapture()
  capture.startAction()
  const fake = fakeItem()
  new Downloads(capture).handle(fake.item)
  assert.equal(fake.cancelled(), true)
  assert.equal(fake.savePath(), '')
  assert.match(
    capture.takeDownloadNotes(),
    /^download cancelled: report\.csv \(8 bytes\) started without download/
  )
  assert.equal(capture.takeDownloadNotes(), '')
})

test('a download the human started in the watch tab keeps its save dialog', () => {
  const capture = newCapture()
  capture.startAction()
  capture.mouseDown()
  const fake = fakeItem()
  new Downloads(capture).handle(fake.item)
  assert.equal(fake.cancelled(), false)
  assert.equal(fake.savePath(), '')
  assert.equal(capture.takeDownloadNotes(), '')
})

test('a download the model asked for gets a save path before the handler returns', async () => {
  const capture = newCapture()
  const downloads = new Downloads(capture)
  const fake = fakeItem()
  const caught = downloads.catchDownload(async () => downloads.handle(fake.item), 'e1')
  assert.notEqual(fake.savePath(), '')
  assert.equal(fake.cancelled(), false)
  fake.finish('interrupted')
  await assert.rejects(caught, /report\.csv was not saved, the download was interrupted/)
})

// @ts-ignore
import assert from 'node:assert/strict'
import { test } from 'node:test'
// @ts-ignore
import { ScrcpyVideoParser, type ScrcpyVideoMeta, type ScrcpyVideoPacket } from './scrcpyStream.ts'

function header(flags: bigint, size: number): Buffer {
  const out = Buffer.alloc(12)
  out.writeBigUInt64BE(flags, 0)
  out.writeUInt32BE(size, 8)
  return out
}

function preamble(): Buffer {
  const name = Buffer.alloc(64)
  name.write('SM-S901N')
  const codec = Buffer.alloc(12)
  codec.write('h264', 0, 'latin1')
  codec.writeUInt32BE(472, 4)
  codec.writeUInt32BE(1024, 8)
  return Buffer.concat([Buffer.from([0]), name, codec])
}

function collect(): {
  parser: ScrcpyVideoParser
  metas: ScrcpyVideoMeta[]
  packets: ScrcpyVideoPacket[]
} {
  const metas: ScrcpyVideoMeta[] = []
  const packets: ScrcpyVideoPacket[] = []
  const parser = new ScrcpyVideoParser({
    meta: (m) => metas.push(m),
    packet: (p) => packets.push(p)
  })
  return { parser, metas, packets }
}

test('reads the device name, codec and size, then config and key packets', () => {
  const { parser, metas, packets } = collect()
  const sps = Buffer.from([0, 0, 0, 1, 0x67, 0x64, 0, 0x20])
  const frame = Buffer.from([0, 0, 0, 1, 0x65, 1, 2, 3])
  parser.push(
    Buffer.concat([
      preamble(),
      header(1n << 63n, sps.length),
      sps,
      header((1n << 62n) | 12345n, frame.length),
      frame
    ])
  )
  assert.deepEqual(metas, [{ name: 'SM-S901N', codec: 'h264', width: 472, height: 1024 }])
  assert.equal(packets.length, 2)
  assert.deepEqual(
    { config: packets[0].config, key: packets[0].key, data: [...packets[0].data] },
    { config: true, key: false, data: [...sps] }
  )
  assert.deepEqual(
    { config: packets[1].config, key: packets[1].key, pts: packets[1].pts },
    { config: false, key: true, pts: 12345 }
  )
})

test('a stream split at every byte parses the same', () => {
  const { parser, metas, packets } = collect()
  const frame = Buffer.from([9, 8, 7, 6, 5])
  const whole = Buffer.concat([preamble(), header(77n, frame.length), frame])
  for (const byte of whole) parser.push(Buffer.from([byte]))
  assert.equal(metas.length, 1)
  assert.equal(packets.length, 1)
  assert.deepEqual([...packets[0].data], [...frame])
  assert.equal(packets[0].pts, 77)
})

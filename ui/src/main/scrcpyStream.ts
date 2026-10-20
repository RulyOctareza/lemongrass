// The video socket of a scrcpy-server started with tunnel_forward=true. The layout matches the
// server shipped with scrcpy 3.3; newer servers mark packets differently, so a version bump needs
// this parser checked against the device first.
const DUMMY_BYTES = 1
const DEVICE_NAME_BYTES = 64
const CODEC_META_BYTES = 12
const FRAME_HEADER_BYTES = 12
const CONFIG_FLAG = 1n << 63n
const KEY_FLAG = 1n << 62n
const PTS_MASK = KEY_FLAG - 1n

export interface ScrcpyVideoMeta {
  name: string
  codec: string
  width: number
  height: number
}

export interface ScrcpyVideoPacket {
  config: boolean
  key: boolean
  pts: number
  data: Buffer
}

export interface ScrcpyVideoHandlers {
  meta: (meta: ScrcpyVideoMeta) => void
  packet: (packet: ScrcpyVideoPacket) => void
}

type Stage = 'dummy' | 'name' | 'codec' | 'header' | 'payload'

export class ScrcpyVideoParser {
  private buffer: Buffer = Buffer.alloc(0)
  private stage: Stage = 'dummy'
  private name = ''
  private pending: Omit<ScrcpyVideoPacket, 'data'> & { size: number } = {
    config: false,
    key: false,
    pts: 0,
    size: 0
  }

  private readonly handlers: ScrcpyVideoHandlers

  constructor(handlers: ScrcpyVideoHandlers) {
    this.handlers = handlers
  }

  push(chunk: Buffer): void {
    this.buffer = this.buffer.length ? Buffer.concat([this.buffer, chunk]) : chunk
    while (this.step()) {
      // Each step consumes one complete unit and reports whether the next can be tried.
    }
  }

  private take(bytes: number): Buffer | null {
    if (this.buffer.length < bytes) return null
    const out = this.buffer.subarray(0, bytes)
    this.buffer = this.buffer.subarray(bytes)
    return out
  }

  private step(): boolean {
    switch (this.stage) {
      case 'dummy':
        if (!this.take(DUMMY_BYTES)) return false
        this.stage = 'name'
        return true
      case 'name': {
        const raw = this.take(DEVICE_NAME_BYTES)
        if (!raw) return false
        const end = raw.indexOf(0)
        this.name = raw.subarray(0, end === -1 ? raw.length : end).toString('utf8')
        this.stage = 'codec'
        return true
      }
      case 'codec': {
        const raw = this.take(CODEC_META_BYTES)
        if (!raw) return false
        this.handlers.meta({
          name: this.name,
          codec: raw.subarray(0, 4).toString('latin1').replace(/^\0+/, ''),
          width: raw.readUInt32BE(4),
          height: raw.readUInt32BE(8)
        })
        this.stage = 'header'
        return true
      }
      case 'header': {
        const raw = this.take(FRAME_HEADER_BYTES)
        if (!raw) return false
        const flags = raw.readBigUInt64BE(0)
        this.pending = {
          config: (flags & CONFIG_FLAG) !== 0n,
          key: (flags & KEY_FLAG) !== 0n,
          pts: Number(flags & PTS_MASK),
          size: raw.readUInt32BE(8)
        }
        this.stage = 'payload'
        return true
      }
      case 'payload': {
        const raw = this.take(this.pending.size)
        if (!raw) return false
        const { config, key, pts } = this.pending
        // Copied so the packet does not pin the whole receive buffer it was sliced from.
        this.handlers.packet({ config, key, pts, data: Buffer.from(raw) })
        this.stage = 'header'
        return true
      }
    }
  }
}

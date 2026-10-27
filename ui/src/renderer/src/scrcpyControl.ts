// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Control messages for the scrcpy 3.3 server, laid out as its client's control_msg.c writes them.
// Positions carry the video frame size; the server drops an event whose size no longer matches
// the frame it is sending, such as one sent across a rotation.
const TYPE_KEYCODE = 0
const TYPE_TEXT = 1
const TYPE_TOUCH = 2
const TYPE_SCROLL = 3

// The server's text limit, in UTF-8 bytes.
const TEXT_MAX_BYTES = 300
const GENERIC_FINGER = 0xfffffffffffffffen

export const TouchAction = { down: 0, up: 1, move: 2 } as const
export const KeyAction = { down: 0, up: 1 } as const

// Android KeyEvent codes for the keys the pane forwards.
export const AndroidKey = {
  home: 3,
  back: 4,
  dpadUp: 19,
  dpadDown: 20,
  dpadLeft: 21,
  dpadRight: 22,
  tab: 61,
  enter: 66,
  del: 67,
  escape: 111,
  forwardDel: 112,
  appSwitch: 187
} as const

export interface FramePoint {
  x: number
  y: number
  width: number
  height: number
}

function writePosition(view: DataView, offset: number, point: FramePoint): void {
  view.setInt32(offset, Math.round(point.x))
  view.setInt32(offset + 4, Math.round(point.y))
  view.setUint16(offset + 8, point.width)
  view.setUint16(offset + 10, point.height)
}

export function touchMessage(action: number, point: FramePoint): Uint8Array {
  const out = new Uint8Array(32)
  const view = new DataView(out.buffer)
  out[0] = TYPE_TOUCH
  out[1] = action
  view.setBigUint64(2, GENERIC_FINGER)
  writePosition(view, 10, point)
  view.setUint16(22, action === TouchAction.up ? 0 : 0xffff)
  view.setUint32(24, 0)
  view.setUint32(28, 0)
  return out
}

export function keyMessage(action: number, keycode: number, metastate = 0): Uint8Array {
  const out = new Uint8Array(14)
  const view = new DataView(out.buffer)
  out[0] = TYPE_KEYCODE
  out[1] = action
  view.setUint32(2, keycode)
  view.setUint32(6, 0)
  view.setUint32(10, metastate)
  return out
}

// Splits on character boundaries so no message carries half of a multi-byte character.
export function textMessages(text: string): Uint8Array[] {
  const encoder = new TextEncoder()
  const messages: Uint8Array[] = []
  let chunk = ''
  const flush = (): void => {
    if (!chunk) return
    const bytes = encoder.encode(chunk)
    const out = new Uint8Array(5 + bytes.length)
    out[0] = TYPE_TEXT
    new DataView(out.buffer).setUint32(1, bytes.length)
    out.set(bytes, 5)
    messages.push(out)
    chunk = ''
  }
  for (const char of text) {
    if (encoder.encode(chunk + char).length > TEXT_MAX_BYTES) flush()
    chunk += char
  }
  flush()
  return messages
}

function toI16FixedPoint(value: number): number {
  const clamped = Math.max(-1, Math.min(1, value))
  return clamped >= 1 ? 0x7fff : Math.round(clamped * 0x8000)
}

// scrcpy accepts scroll amounts in [-16, 16] and sends them normalized to [-1, 1].
export function scrollMessage(point: FramePoint, hscroll: number, vscroll: number): Uint8Array {
  const out = new Uint8Array(21)
  const view = new DataView(out.buffer)
  out[0] = TYPE_SCROLL
  writePosition(view, 1, point)
  view.setInt16(13, toI16FixedPoint(hscroll / 16))
  view.setInt16(15, toI16FixedPoint(vscroll / 16))
  view.setUint32(17, 0)
  return out
}

<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import AppButton from './AppButton.vue'
import { avcCodecString, concatBytes } from '../h264'
import {
  AndroidKey,
  KeyAction,
  TouchAction,
  keyMessage,
  scrollMessage,
  textMessages,
  touchMessage,
  type FramePoint
} from '../scrcpyControl'
import type { DeviceInfo, DevicePacket } from '../../../preload/types'

type Phase = 'picking' | 'starting' | 'live' | 'reconnecting' | 'ended'

// adb's USB link to a phone can drop and come back many times in a row for minutes, so the
// pane keeps waiting for as long as it is open, polling less often the longer it waits.
const RECONNECT_POLL_MS = 1_000
const RECONNECT_POLL_MAX_MS = 5_000
// A second decode error this soon after a restart ends the pane, so a stream that never
// decodes does not restart forever.
const DECODE_RETRY_WINDOW_MS = 10_000

const phase = ref<Phase>('picking')
const devices = ref<DeviceInfo[]>([])
const listError = ref('')
const streamError = ref('')
const current = ref<DeviceInfo | null>(null)
const canvasEl = ref<HTMLCanvasElement>()
const screenEl = ref<HTMLDivElement>()

let sessionId: string | null = null
let decoder: VideoDecoder | null = null
let codec: string | null = null
// scrcpy sends SPS/PPS as their own packet; the decoder needs them in front of the next frame.
let pendingConfig: Uint8Array | null = null
let touching = false
let stopPackets: (() => void) | undefined
let stopEnd: (() => void) | undefined
let reconnectTimer: ReturnType<typeof setTimeout> | undefined
// While set, a failed start goes back to waiting instead of ending the pane.
let reconnecting = false
let lastDecodeRestart = 0

async function refresh(): Promise<void> {
  const result = await window.api.device.list()
  if (!result.ok) {
    listError.value = result.error
    devices.value = []
    return
  }
  listError.value = ''
  devices.value = result.devices
}

function draw(frame: VideoFrame): void {
  const canvas = canvasEl.value
  if (canvas) {
    if (canvas.width !== frame.displayWidth) canvas.width = frame.displayWidth
    if (canvas.height !== frame.displayHeight) canvas.height = frame.displayHeight
    canvas.getContext('2d')?.drawImage(frame, 0, 0)
  }
  frame.close()
}

function onPacket(packet: DevicePacket): void {
  if (packet.id !== sessionId) return
  if (packet.config) {
    const next = avcCodecString(packet.data)
    if (next && next !== codec) {
      codec = next
      decoder?.close()
      decoder = new VideoDecoder({
        output: draw,
        error: (error) => onDecodeError(error.message)
      })
      // With no description the decoder reads Annex B, which is what scrcpy sends.
      decoder.configure({ codec, optimizeForLatency: true })
    }
    pendingConfig = packet.data
    return
  }
  if (!decoder || decoder.state !== 'configured') return
  const data = pendingConfig ? concatBytes(pendingConfig, packet.data) : packet.data
  pendingConfig = null
  decoder.decode(
    new EncodedVideoChunk({
      type: packet.key ? 'key' : 'delta',
      timestamp: packet.pts,
      data
    })
  )
}

function teardown(): void {
  clearTimeout(reconnectTimer)
  reconnectTimer = undefined
  stopPackets?.()
  stopEnd?.()
  stopPackets = stopEnd = undefined
  if (sessionId) window.api.device.stop(sessionId)
  sessionId = null
  if (decoder && decoder.state !== 'closed') decoder.close()
  decoder = null
  codec = null
  pendingConfig = null
}

function fail(message: string): void {
  teardown()
  reconnecting = false
  streamError.value = message
  phase.value = 'ended'
}

function reconnect(device: DeviceInfo, reason: string): void {
  teardown()
  reconnecting = true
  streamError.value = reason
  phase.value = 'reconnecting'
  let delay = RECONNECT_POLL_MS
  const poll = async (): Promise<void> => {
    reconnectTimer = undefined
    const result = await window.api.device.list()
    if (phase.value !== 'reconnecting' || reconnectTimer) return
    const back =
      result.ok &&
      result.devices.some((entry) => entry.serial === device.serial && entry.state === 'device')
    if (back) return void start(device)
    delay = Math.min(delay * 2, RECONNECT_POLL_MAX_MS)
    reconnectTimer = setTimeout(poll, delay)
  }
  reconnectTimer = setTimeout(poll, delay)
}

// A fresh session starts with a key frame, which is what a decoder needs after an error.
function onDecodeError(message: string): void {
  const device = current.value
  const now = Date.now()
  if (!device || now - lastDecodeRestart < DECODE_RETRY_WINDOW_MS) {
    return fail(`Decoding failed: ${message}`)
  }
  lastDecodeRestart = now
  void start(device)
}

async function start(device: DeviceInfo): Promise<void> {
  teardown()
  current.value = device
  streamError.value = ''
  phase.value = 'starting'
  const id = crypto.randomUUID()
  sessionId = id
  stopPackets = window.api.device.onPacket(onPacket)
  stopEnd = window.api.device.onEnd((end) => {
    if (end.id !== sessionId) return
    if (end.replaced) return fail('This phone is now mirrored in another Device tab.')
    reconnect(device, end.error ?? 'The device disconnected.')
  })
  const result = await window.api.device.start({ id, serial: device.serial })
  if (sessionId !== id) return
  if (!result.ok) return reconnecting ? reconnect(device, result.error) : fail(result.error)
  reconnecting = false
  phase.value = 'live'
}

function send(message: Uint8Array): void {
  if (sessionId && phase.value === 'live') window.api.device.control(sessionId, message)
}

function pressKey(keycode: number): void {
  send(keyMessage(KeyAction.down, keycode))
  send(keyMessage(KeyAction.up, keycode))
}

// Maps a pointer on the scaled canvas to a pixel of the video frame.
function framePoint(event: MouseEvent): FramePoint | null {
  const canvas = canvasEl.value
  if (!canvas || !canvas.width) return null
  const rect = canvas.getBoundingClientRect()
  const x = ((event.clientX - rect.left) * canvas.width) / rect.width
  const y = ((event.clientY - rect.top) * canvas.height) / rect.height
  return {
    x: Math.max(0, Math.min(canvas.width - 1, x)),
    y: Math.max(0, Math.min(canvas.height - 1, y)),
    width: canvas.width,
    height: canvas.height
  }
}

function onPointerDown(event: PointerEvent): void {
  screenEl.value?.focus()
  // A secondary click is Back, as in scrcpy.
  if (event.button === 2) return pressKey(AndroidKey.back)
  if (event.button !== 0) return
  const point = framePoint(event)
  if (!point) return
  canvasEl.value?.setPointerCapture(event.pointerId)
  touching = true
  send(touchMessage(TouchAction.down, point))
}

function onPointerMove(event: PointerEvent): void {
  if (!touching) return
  const point = framePoint(event)
  if (point) send(touchMessage(TouchAction.move, point))
}

function onPointerUp(event: PointerEvent): void {
  if (!touching) return
  touching = false
  const point = framePoint(event)
  if (point) send(touchMessage(TouchAction.up, point))
}

function onWheel(event: WheelEvent): void {
  const point = framePoint(event)
  if (!point) return
  send(scrollMessage(point, -event.deltaX / 100, -event.deltaY / 100))
}

const KEYS: Record<string, number> = {
  Enter: AndroidKey.enter,
  Backspace: AndroidKey.del,
  Delete: AndroidKey.forwardDel,
  Tab: AndroidKey.tab,
  Escape: AndroidKey.escape,
  ArrowUp: AndroidKey.dpadUp,
  ArrowDown: AndroidKey.dpadDown,
  ArrowLeft: AndroidKey.dpadLeft,
  ArrowRight: AndroidKey.dpadRight
}

function onKeyDown(event: KeyboardEvent): void {
  // Shortcuts stay with the app, and a paste arrives through onPaste instead.
  if (event.isComposing || event.metaKey || event.ctrlKey || event.altKey) return
  const keycode = KEYS[event.key]
  if (keycode !== undefined) {
    event.preventDefault()
    return pressKey(keycode)
  }
  if (event.key.length === 1) {
    event.preventDefault()
    for (const message of textMessages(event.key)) send(message)
  }
}

function onPaste(event: ClipboardEvent): void {
  const text = event.clipboardData?.getData('text/plain')
  if (!text) return
  event.preventDefault()
  for (const message of textMessages(text)) send(message)
}

function backToList(): void {
  teardown()
  reconnecting = false
  current.value = null
  phase.value = 'picking'
  void refresh()
}

onMounted(async () => {
  await refresh()
  const ready = devices.value.filter((device) => device.state === 'device')
  if (ready.length === 1 && phase.value === 'picking') void start(ready[0])
})

onBeforeUnmount(teardown)
</script>

<template>
  <div class="device-view fade-in">
    <div class="toolbar">
      <span class="device-name">{{ current?.model ?? 'Device' }}</span>
      <span v-if="current" class="device-serial">{{ current.serial }}</span>
      <span class="spacer" />
      <template v-if="phase === 'live'">
        <AppButton title="Back" @click="pressKey(AndroidKey.back)">Back</AppButton>
        <AppButton title="Home" @click="pressKey(AndroidKey.home)">Home</AppButton>
        <AppButton title="Recent apps" @click="pressKey(AndroidKey.appSwitch)">Recents</AppButton>
      </template>
      <AppButton v-if="phase !== 'picking'" @click="backToList">Devices</AppButton>
    </div>

    <div v-if="phase === 'picking'" class="picker">
      <p v-if="listError" class="error">{{ listError }}</p>
      <p v-else-if="devices.length === 0" class="hint">
        No Android device found. Connect a phone over USB with USB debugging on.
      </p>
      <button
        v-for="device in devices"
        :key="device.serial"
        class="device-row"
        :disabled="device.state !== 'device'"
        @click="start(device)"
      >
        <span class="row-model">{{ device.model }}</span>
        <span class="row-serial">{{ device.serial }}</span>
        <span v-if="device.state !== 'device'" class="row-state">{{ device.state }}</span>
      </button>
      <AppButton @click="refresh">Refresh</AppButton>
    </div>

    <div
      v-else
      ref="screenEl"
      class="screen"
      tabindex="0"
      @keydown="onKeyDown"
      @paste="onPaste"
      @contextmenu.prevent
    >
      <canvas
        v-show="phase === 'live'"
        ref="canvasEl"
        class="screen-canvas"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
        @wheel.prevent="onWheel"
      />
      <p v-if="phase === 'starting'" class="hint">Starting the mirror…</p>
      <div v-if="phase === 'reconnecting'" class="ended">
        <p class="error">{{ streamError }}</p>
        <p class="hint">Waiting for the phone to come back…</p>
      </div>
      <div v-if="phase === 'ended'" class="ended">
        <p class="error">{{ streamError }}</p>
        <AppButton v-if="current" variant="primary" @click="start(current)">Reconnect</AppButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.device-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--color-surface-0);
}

.toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border-subtle);
  font-size: var(--text-xs);
}

.device-name {
  color: var(--color-fg-primary);
  font-weight: var(--weight-semibold);
}

.device-serial {
  color: var(--color-fg-secondary);
}

.spacer {
  flex: 1;
}

.picker {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-4);
}

.device-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  max-width: 420px;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
}

.device-row:hover:not(:disabled) {
  background: var(--color-surface-3);
}

.device-row:disabled {
  cursor: default;
  opacity: 0.6;
}

.row-serial,
.row-state {
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.row-state {
  margin-left: auto;
}

.screen {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-3);
}

.screen:focus {
  outline: none;
}

.screen-canvas {
  touch-action: none;
  cursor: pointer;
  max-width: 100%;
  max-height: 100%;
  border-radius: var(--radius-md);
}

.ended {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-3);
}

.hint {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.error {
  max-width: 480px;
  color: var(--color-coral);
  font-size: var(--text-sm);
  white-space: pre-wrap;
}
</style>

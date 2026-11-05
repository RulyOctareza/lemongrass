// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { ITerminalOptions } from '@xterm/xterm'

// The font and colors every xterm in the app draws with, read from the design tokens at creation.

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

const FONT_SIZE = 13
const FONT_WAIT_MS = 3000
let fontReady: Promise<unknown> | undefined

// xterm measures its cell once, at open, from whichever face is available then, and never again
// when a font finishes loading. A terminal opened on the fallback face keeps a row count that no
// longer fits once the real one draws. The timeout keeps a font that fails to load from blocking
// every terminal.
export function terminalFontReady(): Promise<unknown> {
  fontReady ??= Promise.race([
    document.fonts.load(`${FONT_SIZE}px ${cssVar('--font-mono')}`),
    new Promise((resolve) => setTimeout(resolve, FONT_WAIT_MS))
  ])
  return fontReady
}

// background overrides the default surface, for a terminal set inside a surface of its own color.
export function terminalLook(background = cssVar('--color-surface-1')): ITerminalOptions {
  return {
    fontFamily: cssVar('--font-mono'),
    fontSize: FONT_SIZE,
    fontWeight: 400,
    fontWeightBold: 600,
    theme: {
      background,
      foreground: cssVar('--color-fg-primary'),
      cursor: cssVar('--color-amber'),
      cursorAccent: background,
      selectionBackground: cssVar('--color-amber-muted'),
      black: cssVar('--color-gray-700'),
      red: cssVar('--color-coral-dim'),
      green: cssVar('--color-jade-dim'),
      yellow: cssVar('--color-amber-dim'),
      blue: cssVar('--color-sapphire-dim'),
      magenta: cssVar('--color-violet-dim'),
      cyan: cssVar('--color-teal-dim'),
      white: cssVar('--color-gray-100'),
      brightBlack: cssVar('--color-gray-500'),
      brightRed: cssVar('--color-coral'),
      brightGreen: cssVar('--color-jade'),
      brightYellow: cssVar('--color-amber'),
      brightBlue: cssVar('--color-sapphire'),
      brightMagenta: cssVar('--color-violet'),
      brightCyan: cssVar('--color-teal'),
      brightWhite: cssVar('--color-white')
    }
  }
}

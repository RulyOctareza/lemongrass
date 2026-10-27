// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { Marked } from 'marked'

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

const marked = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html: ({ text }) => escapeHtml(text),
    image: ({ href, text }) => `<span class="md-image">${escapeHtml(text || href)}</span>`,
    link({ href, tokens }) {
      const label = this.parser.parseInline(tokens)
      return `<span class="md-link">${label} <span class="md-address">(${escapeHtml(href)})</span></span>`
    }
  }
})

// Renders model-written markdown without producing live HTML: raw HTML is shown as text, images are not loaded and links are not clickable.
export function renderSafeMarkdown(source: string): string {
  return marked.parse(source, { async: false })
}

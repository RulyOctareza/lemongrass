// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { renderSafeMarkdown } from './safeMarkdown.ts'

const hostile = [
  '<script>alert(1)</script>',
  '<img src=x onerror=alert(1)>',
  '[click](javascript:alert(1))',
  '![tracker](https://example.invalid/pixel.png)',
  '<a href="https://example.invalid" onclick="steal()">x</a>',
  '<iframe src="https://example.invalid"></iframe>',
  '**bold** <b onmouseover=alert(1)>hover</b>',
  '[**nested <u>html</u>**](https://example.invalid)',
  '<div style="position:fixed">overlay</div>'
]

test('no markup survives as a live tag or attribute', () => {
  for (const input of hostile) {
    const html = renderSafeMarkdown(input)
    const tags = [...html.matchAll(/<\/?([a-zA-Z][a-zA-Z0-9]*)/g)].map((m) => m[1].toLowerCase())
    for (const tag of tags) {
      assert.ok(
        [
          'p',
          'span',
          'strong',
          'em',
          'br',
          'ul',
          'ol',
          'li',
          'code',
          'pre',
          'h1',
          'h2',
          'h3',
          'h4',
          'h5',
          'h6',
          'blockquote',
          'hr',
          'table',
          'thead',
          'tbody',
          'tr',
          'th',
          'td',
          'del',
          'input'
        ].includes(tag),
        `${input} produced <${tag}>: ${html}`
      )
    }
    assert.ok(
      !/<[^>]+\s(on\w+|href|src|style)\s*=/i.test(html),
      `${input} left an attribute: ${html}`
    )
  }
})

test('links and images become visible text', () => {
  assert.match(
    renderSafeMarkdown('[docs](https://example.invalid/a)'),
    /docs.*\(https:\/\/example\.invalid\/a\)/
  )
  assert.match(renderSafeMarkdown('![alt text](https://example.invalid/p.png)'), /alt text/)
})

test('ordinary structure is laid out', () => {
  const html = renderSafeMarkdown('## Plan\n\n- one\n- two\n\nline a\nline b\n\n`code`')
  assert.match(html, /<h2>Plan<\/h2>/)
  assert.match(html, /<li>one<\/li>/)
  assert.match(html, /line a<br>line b/)
  assert.match(html, /<code>code<\/code>/)
})

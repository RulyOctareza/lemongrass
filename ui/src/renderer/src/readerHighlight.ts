// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorView } from '@codemirror/view'
import { tags as t } from '@lezer/highlight'

// The read-only code reader's look. Colors come from the --color-syntax-* tokens in style.css so
// the palette stays in one place; the rest keeps the plain reader's monospace layout.

const style = HighlightStyle.define([
  { tag: t.keyword, color: 'var(--color-syntax-keyword)' },
  { tag: [t.string, t.character, t.special(t.string)], color: 'var(--color-syntax-string)' },
  { tag: [t.regexp, t.escape], color: 'var(--color-syntax-regexp)' },
  {
    tag: [t.number, t.bool, t.null, t.atom, t.unit, t.constant(t.variableName)],
    color: 'var(--color-syntax-constant)'
  },
  { tag: t.comment, color: 'var(--color-syntax-comment)', fontStyle: 'italic' },
  {
    tag: [
      t.function(t.variableName),
      t.function(t.propertyName),
      t.function(t.definition(t.variableName)),
      t.macroName
    ],
    color: 'var(--color-syntax-function)'
  },
  {
    tag: [t.typeName, t.className, t.namespace, t.standard(t.typeName)],
    color: 'var(--color-syntax-type)'
  },
  { tag: t.tagName, color: 'var(--color-syntax-tag)' },
  { tag: t.attributeName, color: 'var(--color-syntax-attribute)' },
  { tag: [t.propertyName, t.labelName], color: 'var(--color-syntax-property)' },
  {
    tag: [t.operator, t.punctuation, t.bracket, t.separator],
    color: 'var(--color-syntax-punctuation)'
  },
  {
    tag: [t.meta, t.processingInstruction, t.annotation],
    color: 'var(--color-syntax-meta)'
  },
  { tag: t.heading, color: 'var(--color-syntax-heading)', fontWeight: 'bold' },
  { tag: [t.link, t.url], color: 'var(--color-syntax-link)', textDecoration: 'underline' },
  { tag: t.quote, color: 'var(--color-syntax-comment)' },
  { tag: t.monospace, color: 'var(--color-syntax-string)' },
  { tag: t.emphasis, fontStyle: 'italic' },
  { tag: t.strong, fontWeight: 'bold' },
  { tag: t.strikethrough, textDecoration: 'line-through' },
  { tag: t.inserted, color: 'var(--color-syntax-inserted)' },
  { tag: t.deleted, color: 'var(--color-syntax-deleted)' },
  { tag: t.invalid, color: 'var(--color-error)' }
])

const theme = EditorView.theme({
  '&': {
    flex: '1',
    minWidth: '0',
    minHeight: '0',
    fontSize: 'var(--text-sm)',
    color: 'var(--color-fg-primary)',
    backgroundColor: 'transparent'
  },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: 'var(--leading-relaxed)' },
  '.cm-content': { padding: 'var(--space-4) 0' },
  '.cm-line': { padding: '0 var(--space-3)' },
  '.cm-gutters': {
    backgroundColor: 'var(--color-surface-1)',
    color: 'var(--color-fg-muted)',
    borderRight: '1px solid var(--color-border-subtle)'
  },
  '.cm-lineNumbers .cm-gutterElement': { padding: '0 var(--space-3)' },
  '.cm-content ::selection': { backgroundColor: 'var(--color-amber-muted)' }
})

export const readerLook = [theme, syntaxHighlighting(style)]

// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { Extension, type Editor } from '@tiptap/vue-3'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

interface Match {
  from: number
  to: number
}

export interface FindState {
  query: string
  matches: Match[]
  current: number
}

interface FindMeta {
  query?: string
  current?: number
}

const findKey = new PluginKey<FindState>('find')

function findMatches(doc: ProseMirrorNode, query: string): Match[] {
  if (!query) return []
  const needle = query.toLowerCase()
  const matches: Match[] = []
  doc.descendants((node, pos) => {
    if (!node.isTextblock) return true
    let text = ''
    node.forEach((child) => {
      text += child.isText ? child.text! : '￼'.repeat(child.nodeSize)
    })
    const haystack = text.toLowerCase()
    let index = haystack.indexOf(needle)
    while (index !== -1) {
      matches.push({ from: pos + 1 + index, to: pos + 1 + index + needle.length })
      index = haystack.indexOf(needle, index + needle.length)
    }
    return false
  })
  return matches
}

function clampIndex(index: number, length: number): number {
  return length ? ((index % length) + length) % length : 0
}

export const Find = Extension.create({
  name: 'find',

  addProseMirrorPlugins() {
    return [
      new Plugin<FindState>({
        key: findKey,
        state: {
          init: () => ({ query: '', matches: [], current: 0 }),
          apply(tr, prev) {
            const meta = tr.getMeta(findKey) as FindMeta | undefined
            if (!meta && !(tr.docChanged && prev.query)) return prev
            const query = meta?.query ?? prev.query
            const matches = findMatches(tr.doc, query)
            return {
              query,
              matches,
              current: clampIndex(meta?.current ?? prev.current, matches.length)
            }
          }
        },
        props: {
          decorations(state) {
            const find = findKey.getState(state)
            if (!find?.matches.length) return null
            return DecorationSet.create(
              state.doc,
              find.matches.map((match, index) =>
                Decoration.inline(match.from, match.to, {
                  class: index === find.current ? 'find-match current' : 'find-match'
                })
              )
            )
          }
        }
      })
    ]
  }
})

export function findState(editor: Editor): FindState {
  return findKey.getState(editor.state) ?? { query: '', matches: [], current: 0 }
}

function revealCurrent(editor: Editor): void {
  const { matches, current } = findState(editor)
  const match = matches[current]
  if (!match) return
  const { tr } = editor.state
  editor.view.dispatch(
    tr.setSelection(TextSelection.create(tr.doc, match.from, match.to)).scrollIntoView()
  )
}

export function setFindQuery(editor: Editor, query: string): void {
  const { from } = editor.state.selection
  const matches = findMatches(editor.state.doc, query)
  const after = matches.findIndex((match) => match.from >= from)
  editor.view.dispatch(editor.state.tr.setMeta(findKey, { query, current: Math.max(after, 0) }))
  revealCurrent(editor)
}

export function stepFind(editor: Editor, direction: 1 | -1): void {
  const { current } = findState(editor)
  editor.view.dispatch(editor.state.tr.setMeta(findKey, { current: current + direction }))
  revealCurrent(editor)
}

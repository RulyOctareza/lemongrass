// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { FileIcon } from './fileIcons'
import type { IconShape } from './tabKinds'

// The biblio tree's drawn icons, shared by its rows and its search results.
const path = (d: string): IconShape => ({ tag: 'path', attrs: { d } })

const ARCHIVE: IconShape[] = [
  { tag: 'rect', attrs: { x: 1.5, y: 2, width: 11, height: 3, rx: 0.5 } },
  path('M2.5 5v6a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1V5'),
  path('M5.5 7.5h3')
]

// Only the four top-level categories get a distinct icon; anything nested reads as a folder.
const CATEGORY: Record<string, IconShape[]> = {
  books: [
    { tag: 'rect', attrs: { x: 2, y: 1.5, width: 10, height: 11, rx: 1 } },
    path('M5 1.5v11')
  ],
  laws: [path('M7 1 12 3 12 7 7 13 2 7 2 3 Z')],
  handover: [path('M1.5 4.5h9M8 2.5l2.5 2-2.5 2'), path('M12.5 9.5h-9M6 11.5l-2.5-2 2.5-2')],
  scratchpad: [path('M9.5 1.5 12.5 4.5 5 12H2V9z')]
}

const FOLDER: IconShape[] = [path('M1.5 3.5h4l1.5 2h5.5v6h-11z')]

const PAGE: IconShape[] = [path('M3 1.5h5l3 3v8H3z'), path('M8 1.5v3h3')]
const CODE_PAGE: IconShape[] = [...PAGE, path('M6 7.5 4.75 9 6 10.5M8 7.5 9.25 9 8 10.5')]
const IMAGE: IconShape[] = [
  { tag: 'rect', attrs: { x: 1.5, y: 2.5, width: 11, height: 9, rx: 1 } },
  { tag: 'circle', attrs: { cx: 5, cy: 5.5, r: 1 } },
  path('M1.5 10l3.5-3 2.5 2 2-1.5 3 2.5')
]

export const archiveIcon = ARCHIVE

const IMAGE_FILE = /\.(png|jpe?g|gif|webp)$/i

export function biblioDirShapes(path: string): IconShape[] {
  if (!path.includes('/') && CATEGORY[path]) return CATEGORY[path]
  if (path === 'scratchpad/archive') return ARCHIVE
  return FOLDER
}

export function biblioFileShapes(name: string): IconShape[] {
  if (name.endsWith('.md')) return PAGE
  return IMAGE_FILE.test(name) ? IMAGE : CODE_PAGE
}

export function biblioIconOf(path: string, type: 'dir' | 'file'): FileIcon {
  if (type === 'dir') return { shapes: biblioDirShapes(path), color: 'var(--color-fg-secondary)' }
  const name = path.slice(path.lastIndexOf('/') + 1)
  return { shapes: biblioFileShapes(name), color: 'var(--color-fg-muted)' }
}

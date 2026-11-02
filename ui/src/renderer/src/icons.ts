// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { IconShape } from './tabKinds'

// Icon shapes used in more than one place, such as a tab and the empty state of its view. They
// live apart from tabKinds so a view can import one without importing the tab registry, which
// imports every view.
export const historyIcon: IconShape[] = [
  { tag: 'circle', attrs: { cx: 7, cy: 7, r: 5.2 } },
  { tag: 'path', attrs: { d: 'M7 4v3.2l2.2 1.3' } }
]

export const resterClientIcon: IconShape[] = [
  { tag: 'path', attrs: { d: 'M2 4.5h9.5L9.5 2.5' } },
  { tag: 'path', attrs: { d: 'M12 9.5H2.5l2 2' } }
]

export const decisionsIcon: IconShape[] = [
  { tag: 'path', attrs: { d: 'M2.5 2.5h9v6.5h-5l-2.5 2.5v-2.5h-1.5z' } },
  { tag: 'path', attrs: { d: 'M5 5.5l1.4 1.4L9 4.3' } }
]

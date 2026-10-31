// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { computed, nextTick, onMounted, ref, watch, type ComputedRef, type Ref } from 'vue'
import { placeMenu, type MenuAnchor } from './menuPlacement'

// Places a fixed popup menu from its real size. It renders hidden at the window's corner first,
// with a max-content width so it is measured unsqueezed, then moves to where it fits.
export function useMenuPlacement(
  el: Ref<HTMLElement | null>,
  anchor: () => MenuAnchor | null
): ComputedRef<Record<string, string>> {
  const position = ref<{ left: number; top: number } | null>(null)

  function place(): void {
    const at = anchor()
    if (!el.value || !at) {
      position.value = null
      return
    }
    position.value = placeMenu(
      at,
      { width: el.value.offsetWidth, height: el.value.offsetHeight },
      { width: window.innerWidth, height: window.innerHeight }
    )
  }

  onMounted(place)
  watch(anchor, async () => {
    position.value = null
    await nextTick()
    place()
  })

  return computed((): Record<string, string> =>
    position.value
      ? { left: `${position.value.left}px`, top: `${position.value.top}px` }
      : { left: '0px', top: '0px', visibility: 'hidden' }
  )
}

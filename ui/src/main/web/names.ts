import type { Cdp } from './cdp'
import { ACTIONABLE, axText, type AXNode, type DerivedName } from './snapshot'

// Enough for a page full of icon buttons, few enough that a snapshot stays one quick round of calls.
const MAX_DERIVED = 80

// Runs with the element as `this`. The accessibility name is empty here, so look at the markup the way a
// person would: an explicit label, the visible text, an image or svg title. An icon only element with none of
// these gets its tag and the attributes that stay the same between visits, never a class or position.
const DERIVE_NAME = `function () {
  const clean = (text) => (text || '').replace(/\\s+/g, ' ').trim()
  const el = this
  const labelledBy = (el.getAttribute('aria-labelledby') || '').split(/\\s+/)
    .map((id) => document.getElementById(id)).filter(Boolean).map((n) => clean(n.textContent)).join(' ')
  const img = el.querySelector('img[alt]')
  const svgTitle = el.querySelector('svg title')
  const name = clean(el.getAttribute('aria-label')) || clean(labelledBy) || clean(el.getAttribute('title'))
    || clean(el.innerText) || clean(img && img.getAttribute('alt')) || clean(svgTitle && svgTitle.textContent)
    || clean(el.getAttribute('alt')) || clean(el.getAttribute('value'))
  if (name) return { name }
  const bits = [el.tagName.toLowerCase()]
  for (const attr of ['type', 'name', 'role', 'data-testid', 'id']) {
    const value = el.getAttribute(attr)
    if (value) bits.push(attr + '=' + value.slice(0, 40))
  }
  const href = el.getAttribute('href')
  if (href) bits.push('href=' + href.split(/[?#]/)[0].slice(0, 60))
  const use = el.querySelector('svg use')
  const sprite = use && (use.getAttribute('href') || use.getAttribute('xlink:href'))
  const iconClass = Array.from(el.querySelectorAll('i, span, svg')).flatMap((n) => Array.from(n.classList))
    .find((c) => /^(fa|icon|bi|mdi|ti|lucide)[-_]/.test(c))
  if (sprite) bits.push('icon=' + sprite.slice(0, 40))
  else if (iconClass) bits.push('icon=' + iconClass)
  return { hint: bits.join(' ') }
}`

export function needsDerivedName(node: AXNode): boolean {
  return (
    !node.ignored &&
    node.backendDOMNodeId !== undefined &&
    ACTIONABLE.has(axText(node.role)) &&
    axText(node.name) === ''
  )
}

export async function deriveNames(cdp: Cdp, nodes: AXNode[]): Promise<Map<number, DerivedName>> {
  const derived = new Map<number, DerivedName>()
  for (const node of nodes.filter(needsDerivedName).slice(0, MAX_DERIVED)) {
    const backendNodeId = node.backendDOMNodeId as number
    try {
      const { object } = await cdp.send<{ object: { objectId?: string } }>('DOM.resolveNode', {
        backendNodeId
      })
      if (!object.objectId) continue
      const { result } = await cdp.send<{ result: { value?: DerivedName } }>(
        'Runtime.callFunctionOn',
        {
          objectId: object.objectId,
          functionDeclaration: DERIVE_NAME,
          returnByValue: true
        }
      )
      if (result.value) derived.set(backendNodeId, result.value)
    } catch {
      // An element that vanished between the tree and now keeps its empty name.
    }
  }
  return derived
}

import { computeAccessibleName, getRole } from 'dom-accessibility-api'

import type { ElementStyle } from './model.js'

declare global {
  var readStyleParity: () => ElementStyle[]
}

// readStyleParity captures browser styles using the accessible-name algorithm.
globalThis.readStyleParity = function readStyleParity(): ElementStyle[] {
  // Compare resolved CSS properties; custom variables differ between compilers.
  const read = (style: CSSStyleDeclaration): Record<string, string> =>
    Object.fromEntries(
      Array.from(style)
        .filter((property) => !property.startsWith('--'))
        .sort()
        .map((property) => [property, style.getPropertyValue(property)]),
    )
  const pseudo = (element: Element, selector: string) => {
    const style = getComputedStyle(element, selector)
    return style.content === 'none' || style.content === 'normal'
      ? null
      : read(style)
  }
  const pathOf = (element: Element): string => {
    // Locate each element by its tag and sibling position, independent of classes.
    const parent = element.parentElement
    const tag = element.tagName.toLowerCase()
    if (!parent) return tag
    const siblings = Array.from(parent.children).filter(
      (sibling) => sibling.tagName === element.tagName,
    )
    return `${pathOf(parent)}>${tag}:nth-of-type(${siblings.indexOf(element) + 1})`
  }

  // Walk elements without relying on ids or class names that a compiler changes.
  const elements: ElementStyle[] = []
  const walker = document.createTreeWalker(
    document.documentElement,
    NodeFilter.SHOW_ELEMENT,
  )
  let element: Element | null = document.documentElement
  while (element) {
    const style = getComputedStyle(element)
    const box = element.getBoundingClientRect()
    if (
      style.display !== 'none' &&
      style.visibility === 'visible' &&
      box.width > 0 &&
      box.height > 0
    ) {
      elements.push({
        path: pathOf(element),
        role: getRole(element) ?? '',
        name: computeAccessibleName(element),
        box: [box.x, box.y, box.width, box.height],
        styles: read(style),
        before: pseudo(element, '::before'),
        after: pseudo(element, '::after'),
      })
    }
    element = walker.nextNode() as Element | null
  }
  return elements
}

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
      // Include generated content and native control/list pseudo-elements.
      const pseudos: ElementStyle['pseudos'] = {}
      for (const selector of ['::before', '::after']) {
        const generated = getComputedStyle(element, selector)
        if (generated.content !== 'none' && generated.content !== 'normal')
          pseudos[selector] = read(generated)
      }
      const selectors = ['::selection']
      if (element.matches('input[placeholder], textarea[placeholder]'))
        selectors.push('::placeholder')
      if (style.display === 'list-item') selectors.push('::marker')
      if (element.matches('input[type="file"]'))
        selectors.push('::file-selector-button')
      if (element.matches('dialog[open], :popover-open'))
        selectors.push('::backdrop')
      for (const selector of selectors)
        pseudos[selector] = read(getComputedStyle(element, selector))
      elements.push({
        path: pathOf(element),
        role: getRole(element) ?? '',
        name: computeAccessibleName(element),
        box: [box.x, box.y, box.width, box.height],
        styles: read(style),
        pseudos,
      })
    }
    element = walker.nextNode() as Element | null
  }
  return elements
}

import { computeAccessibleName, getRole } from 'dom-accessibility-api'

import type { ElementStyle } from './model.js'

declare global {
  var readStyleParity: () => ElementStyle[]
}

// selectionProperties lists the painted properties that CSS permits on highlights.
const selectionProperties = [
  'color',
  'background-color',
  'text-shadow',
  'text-decoration-color',
  'text-decoration-line',
  'text-decoration-style',
  'text-decoration-thickness',
  '-webkit-text-fill-color',
  '-webkit-text-stroke-color',
  '-webkit-text-stroke-width',
]

// resolveSelectors preserves selector lists and nesting while locating each highlight's originating element.
function resolveSelectors(source: string, parents: string[]) {
  // CSSOM normalizes selectors, but commas inside functions, attributes and escaped identifiers stay literal.
  const parent = parents.length ? `:is(${parents.join(',')})` : ''
  const selectors: Array<{ selector: string; origin: string | null }> = []

  // Track delimiters while keeping the authored selector and its originating-element selector together.
  let selector = ''
  let origin = ''
  let quote = ''
  let depth = 0
  let nested = false
  let highlighted = false
  const finish = () => {
    // Publish one top-level selector before starting its next list entry.
    const prefix = parent && !nested ? parent + ' ' : ''
    selectors.push({
      selector: prefix + selector.trim(),
      origin: highlighted ? prefix + origin.trim() : null,
    })
    selector = ''
    origin = ''
    nested = false
    highlighted = false
  }

  // Rewrite only syntax tokens; quoted and escaped characters cannot introduce a selector or pseudo-element.
  for (let index = 0; index < source.length; index++) {
    const character = source[index]!
    if (character === '\\') {
      const escaped = source.slice(index, index + 2)
      selector += escaped
      origin += escaped
      index++
      continue
    }
    if (quote) {
      selector += character
      origin += character
      if (character === quote) quote = ''
      continue
    }
    if (character === '"' || character === "'") quote = character
    else if (character === '(' || character === '[') depth++
    else if (character === ')' || character === ']') depth--
    else if (character === ',' && depth === 0) {
      finish()
      continue
    } else if (character === '&') {
      selector += parent || ':root'
      origin += parent || ':root'
      nested = true
      continue
    } else if (
      source.slice(index, index + 11).toLowerCase() === '::selection' &&
      !/[\w-]/.test(source[index + 11] ?? '')
    ) {
      selector += '::selection'
      origin += ':is(*)'
      highlighted = true
      index += 10
      continue
    }
    selector += character
    origin += character
  }
  finish()
  return selectors
}

// selectionDeclarations records authored highlight properties on their originating elements.
function selectionDeclarations(): WeakMap<Element, Set<string>> {
  // Browser default highlight styles can retain stale transition colors; ordinary styles
  // already capture that inherited color. Observe explicit highlight declarations instead.
  const declarations = new WeakMap<Element, Set<string>>()
  const visit = (rules: CSSRuleList, parents: string[] = []) => {
    for (const rule of rules) {
      if (
        rule instanceof CSSMediaRule &&
        !matchMedia(rule.conditionText).matches
      )
        continue
      if (rule instanceof CSSSupportsRule && !CSS.supports(rule.conditionText))
        continue
      // Resolve a nested rule against its actual parent selector list before matching highlight origins.
      let descendants = parents
      if (rule instanceof CSSStyleRule) {
        const selectors = resolveSelectors(rule.selectorText, parents)
        descendants = selectors.map(({ selector }) => selector)
        const properties = selectionProperties.filter((property) =>
          rule.style.getPropertyValue(property),
        )
        for (const { origin } of selectors) {
          if (!origin || !properties.length) continue
          for (const element of document.querySelectorAll(origin)) {
            const own = declarations.get(element) ?? new Set<string>()
            for (const property of properties) own.add(property)
            declarations.set(element, own)
          }
        }
      }
      if ('cssRules' in rule)
        visit((rule as CSSGroupingRule).cssRules, descendants)
    }
  }

  // Vite serves application CSS from the fixture origin; unrelated opaque sheets have no inspectable rules.
  for (const sheet of [
    ...document.styleSheets,
    ...document.adoptedStyleSheets,
  ]) {
    try {
      visit(sheet.cssRules)
    } catch (error) {
      if (!(error instanceof DOMException) || error.name !== 'SecurityError')
        throw error
    }
  }
  return declarations
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
  const highlights = selectionDeclarations()
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
      // Highlight inheritance carries authored properties through the ancestor chain.
      const selection = new Set<string>()
      for (
        let origin: Element | null = element;
        origin;
        origin = origin.parentElement
      )
        for (const property of highlights.get(origin) ?? [])
          selection.add(property)
      if (selection.size) {
        const highlight = getComputedStyle(element, '::selection')
        pseudos['::selection'] = Object.fromEntries(
          [...selection]
            .sort()
            .map((property) => [
              property,
              highlight.getPropertyValue(property),
            ]),
        )
      }
      const selectors: string[] = []
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

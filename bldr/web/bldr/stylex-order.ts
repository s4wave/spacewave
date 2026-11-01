import { stylexLayerOrder, stylexPriority } from './stylex-priorities.js'

// orderKey shares one owner even when separately built modules initialize it.
const orderKey = Symbol.for('@aptre/bldr/stylex-order')

// OrderedDocument retains the order for exactly the document's lifetime.
type OrderedDocument = Document & { [orderKey]?: StylexOrder }

// StylexOrder keeps one ordering declaration ahead of every managed stylesheet.
class StylexOrder {
  // priorities retains encountered priorities so removing a sheet cannot reorder others.
  private readonly priorities = new Set<number>()
  // style is the document's first layer-order declaration.
  private readonly style: HTMLStyleElement
  // observer watches Vite's inline stylesheet replacement during development.
  private readonly observer: MutationObserver

  constructor(private readonly document: Document) {
    // Reuse a prerendered declaration, or create the document's ordering point.
    this.style =
      document.head.querySelector<HTMLStyleElement>(
        'style[data-bldr-stylex-order]',
      ) ?? document.createElement('style')
    this.style.setAttribute('data-bldr-stylex-order', '')
    document.head.prepend(this.style)

    // Register already loaded CSS before observing future stylesheet arrivals.
    for (const sheet of document.styleSheets) this.register(sheet)
    document.addEventListener('load', this.onLoad, true)
    this.observer = new MutationObserver(this.onMutation)
    this.observer.observe(document.head, {
      subtree: true,
      childList: true,
      characterData: true,
    })
  }

  // onLoad registers a link before its target handler reveals a WebView.
  private readonly onLoad = (event: Event) => {
    const link = event.target as HTMLLinkElement | null
    if (link?.tagName === 'LINK' && link.rel === 'stylesheet' && link.sheet) {
      this.register(link.sheet)
    }
  }

  // onMutation registers inline CSS added or replaced by Vite's development runtime.
  private readonly onMutation = (records: MutationRecord[]) => {
    // A stylesheet inserted at the start of head must follow the ordering point.
    if (this.document.head.firstChild !== this.style)
      this.document.head.prepend(this.style)

    // Register the changed inline styles, ignoring our own declaration updates.
    for (const record of records) {
      if (
        record.target === this.style ||
        record.target.parentNode === this.style
      )
        continue
      this.registerStyle(record.target)
      for (const node of record.addedNodes) {
        this.registerStyle(node)
        if (node.nodeType === Node.ELEMENT_NODE) {
          for (const style of (node as Element).querySelectorAll('style')) {
            this.registerStyle(style)
          }
        }
      }
    }
  }

  // registerStyle reads an inline stylesheet after its text changes.
  private registerStyle(node: Node): void {
    const element =
      node.nodeType === Node.ELEMENT_NODE ? node : node.parentElement
    if (element?.nodeName === 'STYLE' && element !== this.style) {
      const sheet = (element as HTMLStyleElement).sheet
      if (sheet) this.register(sheet)
    }
  }

  // register publishes newly encountered priorities synchronously before the next paint.
  private register(sheet: CSSStyleSheet): void {
    // Leave unrelated cross-origin stylesheets to their normal browser lifecycle.
    let rules: CSSRuleList
    try {
      rules = sheet.cssRules
    } catch (error) {
      if (
        error &&
        typeof error === 'object' &&
        'name' in error &&
        error.name === 'SecurityError'
      )
        return
      throw error
    }

    // Merge compiler priorities without retaining another copy of stylesheet state.
    let changed = false
    for (const priority of layerPriorities(rules)) {
      if (this.priorities.has(priority)) continue
      this.priorities.add(priority)
      changed = true
    }
    if (changed) this.style.textContent = stylexLayerOrder(this.priorities)
  }
}

// layerPriorities handles qualified and nested layer names after CSS minification.
function* layerPriorities(rules: CSSRuleList, prefix = ''): Generator<number> {
  for (const rule of rules) {
    // Layer ordering statements may declare priorities before any block is present.
    if ('nameList' in rule) {
      for (const name of (rule as CSSLayerStatementRule).nameList) {
        const priority = stylexPriority(prefix + name)
        if (priority !== undefined) yield priority
      }
      continue
    }

    // Preserve the enclosing layer prefix while visiting nested blocks and media rules.
    if (!('cssRules' in rule)) continue
    let nestedPrefix = prefix
    if (rule.cssText.startsWith('@layer ')) {
      const name = (rule as CSSLayerBlockRule).name
      if (name) {
        const priority = stylexPriority(prefix + name)
        if (priority !== undefined) yield priority
        nestedPrefix += `${name}.`
      }
    }
    yield* layerPriorities((rule as CSSGroupingRule).cssRules, nestedPrefix)
  }
}

/** watchStylexOrder maintains compiler priority order for the document's lifetime. */
export function watchStylexOrder(document: Document): void {
  const target = document as OrderedDocument
  target[orderKey] ??= new StylexOrder(document)
}

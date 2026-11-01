/** Viewport identifies one of the sizes used for every parity state. */
export interface Viewport {
  width: number
  height: number
}

/** ElementStyle records an element without depending on its generated classes. */
export interface ElementStyle {
  path: string
  role: string
  name: string
  box: number[]
  styles: Record<string, string>
  before: Record<string, string> | null
  after: Record<string, string> | null
}

/** Snapshot is one settled render of one manifest state. */
export interface Snapshot {
  id: string
  engine: string
  viewport: Viewport
  url: string
  elements: ElementStyle[]
  aria: string
  screenshot: string
}

/** CaptureIndex is published only after every requested state was captured. */
export interface CaptureIndex {
  revision: string
  manifestHash: string
  startedAt: string
  elapsedMs: number
  snapshots: string[]
}

/** Difference identifies the old and new value of one rendered property. */
export interface Difference {
  state: string
  element: string
  property: string
  before: unknown
  after: unknown
}

/** compareSnapshot reports all changed elements, including additions and removals. */
export function* compareSnapshot(
  before: Snapshot,
  after: Snapshot,
): Generator<Difference> {
  // Reject comparisons of different states instead of producing misleading diffs.
  const state = `${before.engine}/${before.viewport.width}x${before.viewport.height}/${before.id}`
  if (
    before.id !== after.id ||
    before.engine !== after.engine ||
    before.viewport.width !== after.viewport.width ||
    before.viewport.height !== after.viewport.height
  ) {
    throw new Error(`Mismatched snapshot: ${state}`)
  }

  // Compare each old element with the element at the same class-independent path.
  const remaining = new Map(
    after.elements.map((element) => [element.path, element]),
  )
  for (const oldElement of before.elements) {
    const newElement = remaining.get(oldElement.path)
    remaining.delete(oldElement.path)
    if (!newElement) {
      yield {
        state,
        element: oldElement.path,
        property: 'element',
        before: oldElement,
        after: null,
      }
      continue
    }
    for (const property of ['role', 'name', 'box'] as const) {
      if (
        JSON.stringify(oldElement[property]) !==
        JSON.stringify(newElement[property])
      ) {
        yield {
          state,
          element: oldElement.path,
          property,
          before: oldElement[property],
          after: newElement[property],
        }
      }
    }
    for (const group of ['styles', 'before', 'after'] as const) {
      const oldStyles = oldElement[group] ?? {}
      const newStyles = newElement[group] ?? {}
      for (const property of new Set([
        ...Object.keys(oldStyles),
        ...Object.keys(newStyles),
      ])) {
        if (oldStyles[property] !== newStyles[property]) {
          yield {
            state,
            element: oldElement.path,
            property: `${group}.${property}`,
            before: oldStyles[property] ?? null,
            after: newStyles[property] ?? null,
          }
        }
      }
    }
  }

  // Report elements that appeared after the conversion.
  for (const element of remaining.values()) {
    yield {
      state,
      element: element.path,
      property: 'element',
      before: null,
      after: element,
    }
  }
  if (before.aria !== after.aria) {
    yield {
      state,
      element: 'body',
      property: 'aria',
      before: before.aria,
      after: after.aria,
    }
  }
  if (before.screenshot !== after.screenshot) {
    yield {
      state,
      element: 'page',
      property: 'screenshot',
      before: before.screenshot,
      after: after.screenshot,
    }
  }
}

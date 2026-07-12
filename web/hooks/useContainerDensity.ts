/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import { useCallback, useEffect, useState } from 'react'

// ContainerDensity is the density tier a container adopts for its own width.
export type ContainerDensity = 'comfortable' | 'compact'

// compactMaxWidth is the inline width, in CSS pixels, at or below which a
// container adopts the compact tier. A pane narrower than this reads as small
// regardless of the viewport, so a grid cell on a large display still compacts.
export const compactMaxWidth = 384

// ContainerDensityState is returned by useContainerDensity. Attach ref to the
// element whose width drives density, then read density.
export interface ContainerDensityState {
  ref: (element: HTMLElement | null) => void
  density: ContainerDensity
  width: number
  measured: boolean
}

// useContainerDensity observes an element's inline width and reports whether its
// content should render at the comfortable or compact tier. Density follows the
// element's own width through a ResizeObserver, never the viewport, so the same
// overlay compacts inside a small pane and relaxes inside a large one. Before
// the first measurement it reports comfortable to avoid flashing compact on a
// wide pane.
export function useContainerDensity(
  threshold = compactMaxWidth,
): ContainerDensityState {
  const [element, setElement] = useState<HTMLElement | null>(null)
  const [width, setWidth] = useState(0)
  const [measured, setMeasured] = useState(false)

  const ref = useCallback((next: HTMLElement | null) => {
    setElement(next)
  }, [])

  useEffect(() => {
    if (!element) return
    const apply = (next: number) => {
      setWidth((current) => (current === next ? current : next))
      setMeasured(true)
    }
    apply(element.getBoundingClientRect().width)
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const inline =
          entry.contentBoxSize?.[0]?.inlineSize ?? entry.contentRect.width
        apply(inline)
      }
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [element])

  const density: ContainerDensity =
    measured && width > 0 && width <= threshold ? 'compact' : 'comfortable'

  return { ref, density, width, measured }
}

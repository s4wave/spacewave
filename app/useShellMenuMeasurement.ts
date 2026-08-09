import { useEffect, useRef } from 'react'
import type { Model } from '@aptre/flex-layout'

const MENU_COLLAPSE_WIDTH = 640

function findTopLeftStrip(container: HTMLElement): HTMLElement | null {
  const strips = Array.from(
    container.querySelectorAll<HTMLElement>(
      '.flexlayout__tabset_tabbar_outer_top',
    ),
  ).filter((element) => !element.closest('.flexlayout__tab'))
  if (strips.length === 0) return null
  return strips.reduce((best, element) => {
    const bounds = element.getBoundingClientRect()
    const bestBounds = best.getBoundingClientRect()
    if (bounds.top < bestBounds.top - 2) return element
    if (bounds.top <= bestBounds.top + 2 && bounds.left < bestBounds.left) {
      return element
    }
    return best
  })
}

export function useShellMenuMeasurement(model: Model) {
  const menuBarRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const menuBar = menuBarRef.current
    const container = containerRef.current
    if (!menuBar || !container) return

    const topLeftStrip = findTopLeftStrip(container)
    const update = () => {
      container.style.setProperty(
        '--menu-bar-width',
        `${menuBar.offsetWidth}px`,
      )
      const width =
        topLeftStrip?.getBoundingClientRect().width ??
        container.getBoundingClientRect().width
      menuBar.dataset.menuCollapsed = String(width < MENU_COLLAPSE_WIDTH)
    }

    update()
    const observer = new ResizeObserver(update)
    observer.observe(menuBar)
    observer.observe(container)
    if (topLeftStrip) observer.observe(topLeftStrip)
    return () => observer.disconnect()
  }, [model])

  return { containerRef, menuBarRef }
}

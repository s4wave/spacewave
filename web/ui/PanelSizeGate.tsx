import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from 'react'

import { cn } from '@s4wave/web/style/utils.js'

export interface PanelSize {
  width: number
  height: number
  measured: boolean
}

interface ElementScale {
  x: number
  y: number
}

interface PanelSizeGateProps extends HTMLAttributes<HTMLDivElement> {
  minWidth?: number
  minHeight?: number
  fallback: ReactNode
  children: ReactNode
}

const defaultSize: PanelSize = {
  width: 0,
  height: 0,
  measured: false,
}

const identityScale: ElementScale = {
  x: 1,
  y: 1,
}

// PanelSizeGate renders a fallback when the panel's unscaled layout size is too small.
export function PanelSizeGate({
  minWidth = 0,
  minHeight = 0,
  fallback,
  children,
  className,
  ...props
}: PanelSizeGateProps) {
  const containerRefValue = useRef<HTMLDivElement | null>(null)
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const [size, setSize] = useState<PanelSize>(defaultSize)
  const setMeasuredSize = useCallback((next: PanelSize) => {
    setSize((current) =>
      (
        current.width === next.width &&
        current.height === next.height &&
        current.measured === next.measured
      ) ?
        current
      : next,
    )
  }, [])
  const containerRef = useCallback(
    (el: HTMLDivElement | null) => {
      containerRefValue.current = el
      setContainer(el)
      if (el) {
        queueMicrotask(() => {
          if (containerRefValue.current === el) {
            setMeasuredSize(measurePanelSize(el))
          }
        })
      }
    },
    [setMeasuredSize],
  )

  useEffect(() => {
    if (!container) return
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setMeasuredSize(measurePanelSize(entry.target, entry.contentRect))
      }
    })
    observer.observe(container)
    return () => observer.disconnect()
  }, [container, setMeasuredSize])

  const tooSmall =
    size.measured &&
    ((minWidth > 0 && size.width < minWidth) ||
      (minHeight > 0 && size.height < minHeight))

  return (
    <div
      {...props}
      ref={containerRef}
      className={cn('h-full min-h-0 w-full min-w-0 overflow-hidden', className)}
    >
      {tooSmall ? fallback : children}
    </div>
  )
}

export function measurePanelSize(
  el: Element,
  observedRect?: DOMRectReadOnly,
): PanelSize {
  const rect = el.getBoundingClientRect()
  const scale = getCumulativeTransformScale(el)
  const rectWidth = scale.x ? rect.width / scale.x : rect.width
  const rectHeight = scale.y ? rect.height / scale.y : rect.height
  const htmlEl = el instanceof HTMLElement ? el : null

  return {
    width: Math.max(
      observedRect?.width ?? 0,
      htmlEl?.clientWidth ?? 0,
      htmlEl?.offsetWidth ?? 0,
      rectWidth,
    ),
    height: Math.max(
      observedRect?.height ?? 0,
      htmlEl?.clientHeight ?? 0,
      htmlEl?.offsetHeight ?? 0,
      rectHeight,
    ),
    measured: true,
  }
}

function getCumulativeTransformScale(el: Element): ElementScale {
  const scale = getElementTransformScale(el)
  if (!el.parentElement) {
    return scale
  }
  const parentScale = getCumulativeTransformScale(el.parentElement)
  return {
    x: scale.x * parentScale.x,
    y: scale.y * parentScale.y,
  }
}

function getElementTransformScale(el: Element): ElementScale {
  const transform = getComputedStyle(el).transform
  if (!transform || transform === 'none') return identityScale
  const parsed = parseTransformScale(transform)
  if (parsed) return parsed
  if (typeof DOMMatrixReadOnly !== 'undefined') {
    const matrix = new DOMMatrixReadOnly(transform)
    return {
      x: Math.hypot(matrix.a, matrix.b),
      y: Math.hypot(matrix.c, matrix.d),
    }
  }
  return identityScale
}

function parseTransformScale(transform: string): ElementScale | null {
  const scaleMatch = transform.match(
    /^scale\(([^,\s)]+)(?:[,\s]+([^,\s)]+))?\)$/,
  )
  if (scaleMatch) {
    const x = Number(scaleMatch[1])
    const y = Number(scaleMatch[2] ?? scaleMatch[1])
    return {
      x: Number.isFinite(x) ? x : 1,
      y: Number.isFinite(y) ? y : 1,
    }
  }

  const matrixMatch = transform.match(/^matrix\((.+)\)$/)
  if (matrixMatch) {
    const values = matrixMatch[1].split(',').map((v) => Number(v.trim()))
    if (values.length === 6) {
      return {
        x: Math.hypot(values[0], values[1]),
        y: Math.hypot(values[2], values[3]),
      }
    }
  }

  const matrix3dMatch = transform.match(/^matrix3d\((.+)\)$/)
  if (matrix3dMatch) {
    const values = matrix3dMatch[1].split(',').map((v) => Number(v.trim()))
    if (values.length === 16) {
      return {
        x: Math.hypot(values[0], values[1], values[2]),
        y: Math.hypot(values[4], values[5], values[6]),
      }
    }
  }

  return null
}

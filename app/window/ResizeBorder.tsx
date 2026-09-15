import React, { useState, useRef, useEffect, useEffectEvent } from 'react'
import { cn } from '@s4wave/web/style/utils.js'

interface ResizeBorderProps {
  direction: 'horizontal' | 'vertical'
  position: 'left' | 'right' | 'top' | 'bottom'
  areaId: string
  onResize: (delta: number) => void
}

interface Coords {
  x: number
  y: number
  width: number
  height: number
}

const HANDLE_SIZE = 10
const HANDLE_OFFSET = HANDLE_SIZE / 2

export function ResizeBorder({
  direction,
  position,
  areaId,
  onResize,
}: ResizeBorderProps) {
  const [isDragging, setIsDragging] = useState(false)
  const [coords, setCoords] = useState<Coords>({
    x: 0,
    y: 0,
    width: 0,
    height: 0,
  })
  const startPosRef = useRef(0)
  const isHorizontal = direction === 'horizontal'
  const resize = useEffectEvent((delta: number) => {
    onResize(delta)
  })

  const updatePosition = useEffectEvent(() => {
    const container = document.querySelector('[data-fluid-layout]')
    const area = container?.querySelector(`[data-area-id="${areaId}"]`)

    if (!container || !area) return

    const containerRect = container.getBoundingClientRect()
    const areaRect = area.getBoundingClientRect()

    setCoords({
      x: areaRect.left - containerRect.left,
      y: areaRect.top - containerRect.top,
      width: areaRect.width,
      height: areaRect.height,
    })
  })

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    updatePosition()
    window.addEventListener('resize', updatePosition)
    return () => window.removeEventListener('resize', updatePosition)
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    updatePosition()
  }, [isDragging])

  useEffect(() => {
    if (!isDragging) return

    const handleMouseMove = (e: MouseEvent) => {
      const currentPos = isHorizontal ? e.clientX : e.clientY
      const delta = currentPos - startPosRef.current
      resize(delta)
      startPosRef.current = currentPos
    }

    const handleMouseUp = () => setIsDragging(false)

    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)

    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging, isHorizontal])

  const handleMouseDown = (e: React.MouseEvent) => {
    e.preventDefault()
    startPosRef.current = isHorizontal ? e.clientX : e.clientY
    setIsDragging(true)
  }

  const positionStyle = isHorizontal
    ? {
        '--resize-border-left': `${position === 'right' ? coords.x + coords.width : coords.x}px`,
        '--resize-border-top': `${coords.y}px`,
        '--resize-border-height': `${coords.height}px`,
        '--resize-border-width': `${HANDLE_SIZE}px`,
        '--resize-border-transform': `translateX(-${HANDLE_OFFSET}px)`,
      }
    : {
        '--resize-border-left': `${coords.x}px`,
        '--resize-border-top': `${position === 'bottom' ? coords.y + coords.height : coords.y}px`,
        '--resize-border-width': `${coords.width}px`,
        '--resize-border-height': `${HANDLE_SIZE}px`,
        '--resize-border-transform': `translateY(-${HANDLE_OFFSET}px)`,
      }

  return (
    <div
      role="separator"
      aria-orientation={isHorizontal ? 'vertical' : 'horizontal'}
      className={cn(
        'resize-border-position absolute z-50',
        isHorizontal ? 'cursor-col-resize' : 'cursor-row-resize',
      )}
      style={positionStyle}
      onMouseDown={handleMouseDown}
    >
      {isDragging && (
        <div
          className={cn(
            'border-editor-border bg-resize-handle-active absolute rounded-menu-button border',
            isHorizontal
              ? 'left-1/2 h-full w-0.5 -translate-x-1/2'
              : 'top-1/2 h-0.5 w-full -translate-y-1/2',
          )}
        />
      )}
    </div>
  )
}

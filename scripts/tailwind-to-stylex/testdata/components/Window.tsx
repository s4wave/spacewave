import React, { ReactNode, useState } from 'react'
import { cn } from '../style/utils.js'

interface WindowProps {
  children: ReactNode
  className?: string
  style?: React.CSSProperties
  'data-area-id'?: string
}

export function Window({
  children,
  className,
  style,
  'data-area-id': dataAreaId,
}: WindowProps) {
  const [isHovered, setIsHovered] = useState(false)

  return (
    <div
      className={cn(
        'hdr-window-glow overflow-hidden rounded-lg border',
        isHovered ? 'border-ui-outline-active' : 'border-window-border',
        className,
      )}
      style={style}
      data-area-id={dataAreaId}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      {children}
    </div>
  )
}

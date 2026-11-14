import type { ComponentPropsWithRef, ReactNode } from 'react'
import { cn } from '../style/utils.js'

export type PanelHeaderVariant =
  | 'default'
  | 'secondary'
  | 'transparent'
  | 'compact'

interface PanelHeaderProps extends ComponentPropsWithRef<'div'> {
  children?: ReactNode
  height?: number
  variant?: PanelHeaderVariant
}

const variantStyles: Record<PanelHeaderVariant, string> = {
  default: 'bg-panel-header',
  secondary: 'bg-outliner-header',
  transparent: 'bg-transparent',
  compact: 'bg-panel-header gap-1.5',
}

/**
 * PanelHeader renders the header row of a panel. An object viewer's own bar is
 * ViewerBar, not a panel header.
 */
export function PanelHeader({
  children,
  className,
  height = 25,
  style: _style,
  variant = 'default',
  ...props
}: PanelHeaderProps) {
  return (
    <div
      {...props}
      data-drag-handle=""
      className={cn(
        'panel-header-height text-ui border-ui-outline flex shrink-0 items-center gap-2 border-b px-2',
        variantStyles[variant],
        className,
      )}
      style={{ '--panel-header-height': `${height}px` }}
    >
      {children}
    </div>
  )
}

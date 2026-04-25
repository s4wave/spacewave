import { forwardRef, useState, type ReactNode } from 'react'
import { cn } from '../style/utils.js'
import { useObjectViewer } from '@s4wave/web/object/ObjectViewerContext.js'
import { ComponentSelector } from '@s4wave/web/object/ComponentSelector.js'

export type PanelHeaderVariant = 'default' | 'secondary' | 'transparent'

interface PanelHeaderProps {
  children?: ReactNode
  className?: string
  height?: number
  variant?: PanelHeaderVariant
}

const variantStyles: Record<PanelHeaderVariant, string> = {
  default: 'bg-panel-header',
  secondary: 'bg-outliner-header',
  transparent: 'bg-transparent',
}

// PanelHeader renders a consistent header bar for panels with optional viewer selector.
export const PanelHeader = forwardRef<HTMLDivElement, PanelHeaderProps>(
  function PanelHeader(
    { children, className, height = 25, variant = 'default' },
    ref,
  ) {
    const [selectorOpen, setSelectorOpen] = useState(false)
    const viewer = useObjectViewer()
    const showSelector = viewer && viewer.visibleComponents.length > 1
    const showStaticName = viewer && viewer.visibleComponents.length === 1

    return (
      <div
        ref={ref}
        data-drag-handle=""
        className={cn(
          'text-ui border-ui-outline flex shrink-0 items-center gap-2 border-b px-2',
          variantStyles[variant],
          className,
        )}
        style={{ height }}
      >
        {showSelector && viewer.selectedComponent && (
          <ComponentSelector
            open={selectorOpen}
            onOpenChange={setSelectorOpen}
            components={viewer.visibleComponents}
            selectedComponent={viewer.selectedComponent}
            onSelectComponent={viewer.onSelectComponent}
          >
            <div className="text-muted-foreground cursor-pointer truncate text-xs">
              {viewer.selectedComponent.name}
            </div>
          </ComponentSelector>
        )}
        {showStaticName && viewer.visibleComponents[0] && (
          <div className="text-muted-foreground truncate text-xs">
            {viewer.visibleComponents[0].name}
          </div>
        )}
        {children}
      </div>
    )
  },
)

interface PanelHeaderButtonProps {
  children: ReactNode
  onClick?: () => void
  className?: string
  title?: string
}

// PanelHeaderButton renders a consistent icon button for panel headers.
export function PanelHeaderButton({
  children,
  onClick,
  className,
  title,
}: PanelHeaderButtonProps) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={cn(
        'hover:bg-pulldown-hover focus-ring flex items-center justify-center rounded p-0.5 transition-colors',
        className,
      )}
    >
      {children}
    </button>
  )
}

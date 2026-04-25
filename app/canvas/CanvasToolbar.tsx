import { useCallback } from 'react'
import {
  LuMousePointer2,
  LuPencil,
  LuType,
  LuSquare,
  LuSquarePlus,
  LuZoomIn,
  LuZoomOut,
  LuMaximize2,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@s4wave/web/ui/tooltip.js'

import type { CanvasTool, CanvasAction } from './types.js'

// CanvasToolbarProps are the props for CanvasToolbar.
interface CanvasToolbarProps {
  tool: CanvasTool
  onToolChange: (tool: CanvasTool) => void
  actions: Record<CanvasAction, () => void>
  // onAddObject opens the object picker to pin an existing world object.
  // When omitted the button is hidden (no pin handler available).
  onAddObject?: () => void
  className?: string
}

// ToolButton renders a single toolbar button with a tooltip.
function ToolButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string
  active?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          className={cn(
            'flex h-8 w-8 items-center justify-center rounded-md transition-colors duration-150',
            active ?
              'bg-foreground/10 text-foreground'
            : 'text-foreground-alt/50 hover:bg-foreground/6 hover:text-foreground-alt',
          )}
          onClick={onClick}
          aria-label={label}
        >
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  )
}

// CanvasToolbar renders the left sidebar with tool and action buttons.
export function CanvasToolbar({
  tool,
  onToolChange,
  actions,
  onAddObject,
  className,
}: CanvasToolbarProps) {
  const setSelect = useCallback(() => onToolChange('select'), [onToolChange])
  const setDraw = useCallback(() => onToolChange('draw'), [onToolChange])
  const setText = useCallback(() => onToolChange('text'), [onToolChange])
  const setObject = useCallback(() => onToolChange('object'), [onToolChange])

  return (
    <div
      className={cn(
        'bg-background-dark/80 border-foreground/6 flex flex-col items-center gap-1 border-r p-1.5 backdrop-blur-sm',
        className,
      )}
    >
      <ToolButton
        label="Select (V)"
        active={tool === 'select'}
        onClick={setSelect}
      >
        <LuMousePointer2 size={16} />
      </ToolButton>
      <ToolButton label="Draw (D)" active={tool === 'draw'} onClick={setDraw}>
        <LuPencil size={16} />
      </ToolButton>
      <ToolButton label="Text (T)" active={tool === 'text'} onClick={setText}>
        <LuType size={16} />
      </ToolButton>
      <ToolButton
        label="Object (O)"
        active={tool === 'object'}
        onClick={setObject}
      >
        <LuSquare size={16} />
      </ToolButton>

      <div className="bg-foreground/6 my-1 h-px w-6" />

      {onAddObject && (
        <ToolButton label="Add Existing Object" onClick={onAddObject}>
          <LuSquarePlus size={16} />
        </ToolButton>
      )}

      <ToolButton label="Zoom In (+)" onClick={actions['zoom-in']}>
        <LuZoomIn size={16} />
      </ToolButton>
      <ToolButton label="Zoom Out (-)" onClick={actions['zoom-out']}>
        <LuZoomOut size={16} />
      </ToolButton>
      <ToolButton label="Fit View" onClick={actions['fit-view']}>
        <LuMaximize2 size={16} />
      </ToolButton>
    </div>
  )
}

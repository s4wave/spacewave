import { useCallback, type ReactNode } from 'react'
import {
  LuLayoutGrid,
  LuMaximize2,
  LuMousePointer2,
  LuPencil,
  LuSquare,
  LuSquarePlus,
  LuType,
  LuZoomIn,
  LuZoomOut,
} from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { CanvasColorPicker } from './CanvasColorPicker.js'
import { DEFAULT_CANVAS_COLOR } from './geometry.js'
import type { CanvasAction, CanvasTool } from './types.js'

interface CanvasToolbarProps {
  tool: CanvasTool
  color?: string
  onToolChange: (tool: CanvasTool) => void
  onColorChange?: (color: string) => void
  actions: Record<CanvasAction, () => void>
  onAddObject?: () => void
  className?: string
}

function ToolButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string
  active?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className={cn(
            'flex h-8 w-8 items-center justify-center rounded-md transition-colors duration-150',
            active
              ? 'bg-foreground/10 text-foreground'
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

// CanvasToolbar renders the drawing, insertion, and viewport controls.
export function CanvasToolbar({
  tool,
  color = DEFAULT_CANVAS_COLOR,
  onToolChange,
  onColorChange,
  actions,
  onAddObject,
  className,
}: CanvasToolbarProps) {
  const setSelect = useCallback(() => onToolChange('select'), [onToolChange])
  const setDraw = useCallback(() => onToolChange('draw'), [onToolChange])
  const setText = useCallback(() => onToolChange('text'), [onToolChange])
  const setObject = useCallback(() => onToolChange('object'), [onToolChange])
  const handleColorChange = useCallback(
    (nextColor: string) => onColorChange?.(nextColor),
    [onColorChange],
  )

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

      <CanvasColorPicker color={color} onColorChange={handleColorChange} />

      {onAddObject && (
        <ToolButton label="Add Existing Object" onClick={onAddObject}>
          <LuSquarePlus size={16} />
        </ToolButton>
      )}

      <div className="bg-foreground/6 my-1 h-px w-6" />

      <ToolButton label="Zoom In (+)" onClick={actions['zoom-in']}>
        <LuZoomIn size={16} />
      </ToolButton>
      <ToolButton label="Zoom Out (-)" onClick={actions['zoom-out']}>
        <LuZoomOut size={16} />
      </ToolButton>
      <ToolButton label="Fit View" onClick={actions['fit-view']}>
        <LuMaximize2 size={16} />
      </ToolButton>
      <ToolButton label="Organize Nodes" onClick={actions['organize-nodes']}>
        <LuLayoutGrid size={16} />
      </ToolButton>
    </div>
  )
}

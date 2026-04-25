import { useCallback } from 'react'
import {
  LuClipboardPaste,
  LuMaximize2,
  LuScan,
  LuSquare,
  LuType,
} from 'react-icons/lu'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownMenuGhostAnchor } from '@s4wave/web/ui/DropdownMenuGhostAnchor.js'

// CanvasContextMenuState stores the menu anchor in screen and canvas space.
export interface CanvasContextMenuState {
  position: { x: number; y: number }
}

// CanvasContextMenuProps configures the canvas background context menu.
export interface CanvasContextMenuProps {
  state: CanvasContextMenuState | null
  canAddObject: boolean
  onClose: () => void
  onPaste: () => void
  onAddText: () => void
  onAddObject: () => void
  onFitView: () => void
  onZoomReset: () => void
  onSelectAll: () => void
}

// CanvasContextMenu renders the canvas background right-click menu.
export function CanvasContextMenu({
  state,
  canAddObject,
  onClose,
  onPaste,
  onAddText,
  onAddObject,
  onFitView,
  onZoomReset,
  onSelectAll,
}: CanvasContextMenuProps) {
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        onClose()
      }
    },
    [onClose],
  )

  return (
    <DropdownMenu open={state !== null} onOpenChange={handleOpenChange}>
      <DropdownMenuTrigger asChild>
        <DropdownMenuGhostAnchor
          x={state?.position.x ?? 0}
          y={state?.position.y ?? 0}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        side="bottom"
        onCloseAutoFocus={(e) => {
          e.preventDefault()
        }}
      >
        <DropdownMenuItem onSelect={onPaste}>
          <LuClipboardPaste className="h-3.5 w-3.5" />
          Paste
          <DropdownMenuShortcut>Cmd+V</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onAddText}>
          <LuType className="h-3.5 w-3.5" />
          Add Text Node Here
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onAddObject} disabled={!canAddObject}>
          <LuSquare className="h-3.5 w-3.5" />
          Add Object to Canvas
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onFitView}>
          <LuMaximize2 className="h-3.5 w-3.5" />
          Fit View
          <DropdownMenuShortcut>Cmd+0</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onZoomReset}>
          <LuScan className="h-3.5 w-3.5" />
          Zoom to 100%
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onSelectAll}>
          <LuSquare className="h-3.5 w-3.5" />
          Select All
          <DropdownMenuShortcut>Cmd+A</DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

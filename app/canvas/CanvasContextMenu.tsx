import { useCallback } from 'react'
import {
  LuClipboardPaste,
  LuMaximize2,
  LuScan,
  LuSquare,
  LuType,
} from '@s4wave/web/images/icons.js'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownMenuGhostAnchor } from '@s4wave/web/ui/DropdownMenuGhostAnchor.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

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
          <LuClipboardPaste sx={stylexStyles.rule1} />
          Paste
          <DropdownMenuShortcut>Cmd+V</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onAddText}>
          <LuType sx={stylexStyles.rule1} />
          Add Text Node Here
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onAddObject} disabled={!canAddObject}>
          <LuSquare sx={stylexStyles.rule1} />
          Add Object to Canvas
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={onFitView}>
          <LuMaximize2 sx={stylexStyles.rule1} />
          Fit View
          <DropdownMenuShortcut>Cmd+0</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onZoomReset}>
          <LuScan sx={stylexStyles.rule1} />
          Zoom to 100%
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={onSelectAll}>
          <LuSquare sx={stylexStyles.rule1} />
          Select All
          <DropdownMenuShortcut>Cmd+A</DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

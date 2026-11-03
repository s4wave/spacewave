import * as stylex from '@stylexjs/stylex'
import { useRef, type ReactNode } from 'react'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownMenuGhostAnchor } from '@s4wave/web/ui/DropdownMenuGhostAnchor.js'

// FlexTabContextMenuState stores the clicked tab and screen position.
export interface FlexTabContextMenuState {
  tabId: string
  x: number
  y: number
}

// FlexTabContextMenuItem describes one tab context menu action.
export interface FlexTabContextMenuItem {
  id: string
  label: string
  icon?: ReactNode
  disabled?: boolean
  variant?: 'destructive'
  onSelect: (tabId: string) => void
}

export type FlexTabContextMenuEntry =
  | FlexTabContextMenuItem
  | { id: string; type: 'separator' }

// FlexTabContextMenuProps configures the shared FlexLayout tab context menu.
export interface FlexTabContextMenuProps {
  sx?: stylex.StyleXStyles
  className?: never
  state: FlexTabContextMenuState | null
  items: FlexTabContextMenuEntry[]
  onClose: () => void
}

function isSeparator(
  item: FlexTabContextMenuEntry,
): item is { id: string; type: 'separator' } {
  return 'type' in item && item.type === 'separator'
}

// FlexTabContextMenu renders a positioned menu for FlexLayout tabs.
export function FlexTabContextMenu({
  state,
  items,
  onClose,
  sx,
}: FlexTabContextMenuProps) {
  // chosenRef holds the selected action until the menu has closed. Radix
  // commits a selection while the menu still traps focus, so an action that
  // focuses something, such as an inline rename field, would lose that focus
  // back to the menu. onCloseAutoFocus fires once the menu releases focus.
  const chosenRef = useRef<(() => void) | null>(null)

  return (
    <DropdownMenu
      open={state !== null}
      onOpenChange={(open) => {
        if (!open) {
          onClose()
        }
      }}
    >
      <DropdownMenuTrigger asChild>
        <DropdownMenuGhostAnchor x={state?.x ?? 0} y={state?.y ?? 0} />
      </DropdownMenuTrigger>
      {/* The ghost-anchor trigger is not focusable, so skip Radix's return
          of focus to it and run the chosen action instead. */}
      <DropdownMenuContent
        sx={sx}
        align="start"
        side="bottom"
        onCloseAutoFocus={(event) => {
          event.preventDefault()
          const chosen = chosenRef.current
          chosenRef.current = null
          chosen?.()
        }}
      >
        {items.map((item) =>
          isSeparator(item) ? (
            <DropdownMenuSeparator key={item.id} />
          ) : (
            <DropdownMenuItem
              key={item.id}
              onSelect={() => {
                if (!state) return
                const tabId = state.tabId
                chosenRef.current = () => item.onSelect(tabId)
              }}
              disabled={item.disabled}
              variant={item.variant}
            >
              {item.icon}
              {item.label}
            </DropdownMenuItem>
          ),
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

import type { ReactNode } from 'react'

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
}: FlexTabContextMenuProps) {
  function handleAction(action: (tabId: string) => void) {
    if (!state) return
    action(state.tabId)
  }

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
      {/* Inline rename actions mount a tab input as the menu closes. Radix
          returns focus to the trigger during close; suppress that return so the
          new input keeps focus. The ghost-anchor trigger is not focusable. */}
      <DropdownMenuContent
        align="start"
        side="bottom"
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        {items.map((item) =>
          isSeparator(item) ? (
            <DropdownMenuSeparator key={item.id} />
          ) : (
            <DropdownMenuItem
              key={item.id}
              onClick={() => handleAction(item.onSelect)}
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

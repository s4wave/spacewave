import {
  LuCircleX,
  LuCopy,
  LuExternalLink,
  LuPencil,
  LuPlus,
  LuX,
} from 'react-icons/lu'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownMenuGhostAnchor } from '@s4wave/web/ui/DropdownMenuGhostAnchor.js'

// ShellTabContextMenuState stores the clicked tab and screen position.
export interface ShellTabContextMenuState {
  tabId: string
  x: number
  y: number
}

// ShellTabContextMenuProps configures the shared shell tab context menu.
export interface ShellTabContextMenuProps {
  state: ShellTabContextMenuState | null
  canCloseTabs: boolean
  onClose: () => void
  onNewTab: (tabId: string) => void
  onRenameTab: (tabId: string) => void
  onDuplicateTab: (tabId: string) => void
  onPopoutTab: (tabId: string) => void
  onCloseOtherTabs: (tabId: string) => void
  onCloseTab: (tabId: string) => void
}

// ShellTabContextMenu renders the right-click menu for shell tabs.
export function ShellTabContextMenu({
  state,
  canCloseTabs,
  onClose,
  onNewTab,
  onRenameTab,
  onDuplicateTab,
  onPopoutTab,
  onCloseOtherTabs,
  onCloseTab,
}: ShellTabContextMenuProps) {
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
      <DropdownMenuContent align="start" side="bottom">
        <DropdownMenuItem onClick={() => handleAction(onNewTab)}>
          <LuPlus className="h-4 w-4" />
          New Tab
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleAction(onRenameTab)}>
          <LuPencil className="h-4 w-4" />
          Rename Tab
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleAction(onDuplicateTab)}>
          <LuCopy className="h-4 w-4" />
          Duplicate Tab
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => handleAction(onPopoutTab)}>
          <LuExternalLink className="h-4 w-4" />
          Open in New Tab
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => handleAction(onCloseOtherTabs)}
          disabled={!canCloseTabs}
        >
          <LuCircleX className="h-4 w-4" />
          Close Other Tabs
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => handleAction(onCloseTab)}
          disabled={!canCloseTabs}
          variant="destructive"
        >
          <LuX className="h-4 w-4" />
          Close Tab
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

import {
  LuCircleX,
  LuCopy,
  LuExternalLink,
  LuPencil,
  LuPlus,
  LuX,
} from 'react-icons/lu'

import {
  FlexTabContextMenu,
  type FlexTabContextMenuState,
} from '@s4wave/web/layout/FlexTabContextMenu.js'

export type ShellTabContextMenuState = FlexTabContextMenuState

// ShellTabContextMenuProps configures the shell tab context menu binding.
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

// ShellTabContextMenu binds shell tab actions to the shared tab menu.
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
  return (
    <FlexTabContextMenu
      state={state}
      onClose={onClose}
      items={[
        {
          id: 'new-tab',
          label: 'New Tab',
          icon: <LuPlus className="size-4" />,
          onSelect: onNewTab,
        },
        {
          id: 'rename-tab',
          label: 'Rename Tab',
          icon: <LuPencil className="size-4" />,
          onSelect: onRenameTab,
        },
        {
          id: 'duplicate-tab',
          label: 'Duplicate Tab',
          icon: <LuCopy className="size-4" />,
          onSelect: onDuplicateTab,
        },
        {
          id: 'popout-tab',
          label: 'Open in New Tab',
          icon: <LuExternalLink className="size-4" />,
          onSelect: onPopoutTab,
        },
        { id: 'close-separator', type: 'separator' },
        {
          id: 'close-other-tabs',
          label: 'Close Other Tabs',
          icon: <LuCircleX className="size-4" />,
          disabled: !canCloseTabs,
          onSelect: onCloseOtherTabs,
        },
        {
          id: 'close-tab',
          label: 'Close Tab',
          icon: <LuX className="size-4" />,
          disabled: !canCloseTabs,
          variant: 'destructive',
          onSelect: onCloseTab,
        },
      ]}
    />
  )
}

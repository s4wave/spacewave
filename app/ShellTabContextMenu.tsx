import {
  LuCircleX,
  LuCopy,
  LuExternalLink,
  LuPencil,
  LuPlus,
  LuX,
} from '@s4wave/web/images/icons.js'

import {
  FlexTabContextMenu,
  type FlexTabContextMenuState,
} from '@s4wave/web/layout/FlexTabContextMenu.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
          icon: <LuPlus sx={stylexStyles.rule1} />,
          onSelect: onNewTab,
        },
        {
          id: 'rename-tab',
          label: 'Rename Tab',
          icon: <LuPencil sx={stylexStyles.rule1} />,
          onSelect: onRenameTab,
        },
        {
          id: 'duplicate-tab',
          label: 'Duplicate Tab',
          icon: <LuCopy sx={stylexStyles.rule1} />,
          onSelect: onDuplicateTab,
        },
        {
          id: 'popout-tab',
          label: 'Open in New Tab',
          icon: <LuExternalLink sx={stylexStyles.rule1} />,
          onSelect: onPopoutTab,
        },
        { id: 'close-separator', type: 'separator' },
        {
          id: 'close-other-tabs',
          label: 'Close Other Tabs',
          icon: <LuCircleX sx={stylexStyles.rule1} />,
          disabled: !canCloseTabs,
          onSelect: onCloseOtherTabs,
        },
        {
          id: 'close-tab',
          label: 'Close Tab',
          icon: <LuX sx={stylexStyles.rule1} />,
          disabled: !canCloseTabs,
          variant: 'destructive',
          onSelect: onCloseTab,
        },
      ]}
    />
  )
}

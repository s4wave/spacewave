import { getTabDisplayName, type ShellTab } from '@s4wave/app/shell-tab.js'
import { FlexTabNameInput } from '@s4wave/web/layout/FlexTabNameInput.js'

import { useShellTabs } from './ShellTabContext.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
})

// ShellTabLabelProps are the props for ShellTabLabel.
interface ShellTabLabelProps {
  tab: ShellTab
}

// ShellTabLabel renders a tab label that supports inline rename.
// The tab is in rename while the shell tab context's renamingTabId names it;
// double-click or "Rename Tab" from the context menu starts the rename.
// Press Enter or blur to save. Press Escape to cancel.
// Clearing the input reverts to the auto-derived default name.
export function ShellTabLabel({ tab }: ShellTabLabelProps) {
  const { updateTabName, renamingTabId, startRenaming, stopRenaming } =
    useShellTabs()
  const displayName = getTabDisplayName(tab)

  if (renamingTabId === tab.id) {
    return (
      <FlexTabNameInput
        name={displayName}
        label="Tab name"
        onSave={(name) => {
          // Empty string clears custom name, reverting to default
          updateTabName(tab.id, name)
          stopRenaming()
        }}
        onCancel={stopRenaming}
      />
    )
  }

  return (
    <span
      sx={stylexStyles.rule1}
      onDoubleClick={(event) => {
        event.preventDefault()
        event.stopPropagation()
        startRenaming(tab.id)
      }}
    >
      {displayName}
    </span>
  )
}

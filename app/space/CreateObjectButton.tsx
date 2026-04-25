import { useCallback } from 'react'
import { LuPlus } from 'react-icons/lu'

import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { useOpenCommand } from '@s4wave/web/command/CommandContext.js'

// CreateObjectButton renders a plus button in the bottom bar that opens
// the command palette in "Create Object" sub-item mode.
export function CreateObjectButton() {
  const openCommand = useOpenCommand()

  const handleClick = useCallback(() => {
    openCommand('spacewave.create-object')
  }, [openCommand])

  const button = useCallback(
    (_selected: boolean, _onClick: () => void) => (
      <BottomBarItem
        selected={false}
        onClick={handleClick}
        aria-label="Create new object"
      >
        <LuPlus />
      </BottomBarItem>
    ),
    [handleClick],
  )

  return (
    <BottomBarLevel id="create-object" position="right" button={button}>
      {null}
    </BottomBarLevel>
  )
}

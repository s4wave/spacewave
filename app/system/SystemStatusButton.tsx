import { useCallback } from 'react'
import { LuActivity } from 'react-icons/lu'

import { useBottomBarDismissMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { SystemStatusDashboard } from './SystemStatusDashboard.js'

const menuId = 'system-status'

// SystemStatusButton registers a system status dashboard button in the bottom
// bar using BottomBarLevel. Clicking the button toggles a full-page overlay
// showing the SystemStatusDashboard.
export function SystemStatusButton() {
  const dismissMenu = useBottomBarDismissMenu(menuId)

  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => {
      return (
        <BottomBarItem
          selected={selected}
          onClick={onClick}
          className={className}
          aria-label={
            selected ? 'Close system status view' : 'Open system status view'
          }
        >
          <LuActivity aria-hidden="true" />
        </BottomBarItem>
      )
    },
    [],
  )

  return (
    <BottomBarLevel
      id={menuId}
      position="right"
      button={buttonRender}
      overlay={<SystemStatusDashboard onClose={dismissMenu} />}
    >
      {null}
    </BottomBarLevel>
  )
}

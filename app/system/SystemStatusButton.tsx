import { useCallback, useRef } from 'react'
import { LuActivity } from 'react-icons/lu'

import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { SystemStatusDashboard } from './SystemStatusDashboard.js'

// SystemStatusButton registers a system status dashboard button in the bottom
// bar using BottomBarLevel. Clicking the button toggles a full-page overlay
// showing the SystemStatusDashboard.
export function SystemStatusButton() {
  const closeRef = useRef<(() => void) | null>(null)

  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => {
      if (selected) {
        closeRef.current = onClick
      }
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

  const handleClose = useCallback(() => {
    closeRef.current?.()
  }, [])

  return (
    <BottomBarLevel
      id="system-status"
      position="right"
      button={buttonRender}
      overlay={<SystemStatusDashboard onClose={handleClose} />}
    >
      {null}
    </BottomBarLevel>
  )
}

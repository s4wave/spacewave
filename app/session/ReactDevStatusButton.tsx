import { useCallback } from 'react'

import { ReactDevControls } from '@s4wave/web/devtools/ReactDevControls.js'
import { useReactDev } from '@s4wave/web/devtools/react-dev.js'
import { useBottomBarDismissMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'

const menuId = 'react-dev-status'

// ReactDevStatusButton marks a document running Development React in the
// bottom bar, with the setting's controls in its popover.
export function ReactDevStatusButton() {
  const dismissMenu = useBottomBarDismissMenu(menuId)
  const control = useReactDev()
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) =>
      control && (
        <Popover
          open={selected}
          onOpenChange={(open) => (open ? onClick() : dismissMenu())}
        >
          <PopoverTrigger asChild>
            <BottomBarItem
              selected={selected}
              className={className}
              data-testid="react-dev-status-button"
            >
              <span className="text-warning font-mono text-xs">Dev React</span>
            </BottomBarItem>
          </PopoverTrigger>
          <PopoverContent
            side="top"
            align="end"
            sideOffset={6}
            variant="status"
          >
            <div className="p-3">
              <ReactDevControls control={control} />
            </div>
          </PopoverContent>
        </Popover>
      ),
    [control, dismissMenu],
  )

  if (!control?.model.running) return null
  return (
    <BottomBarLevel id={menuId} position="right" button={buttonRender}>
      {null}
    </BottomBarLevel>
  )
}

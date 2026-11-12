import { useCallback } from 'react'

import { ReactDevControls } from '@s4wave/web/devtools/ReactDevControls.js'
import { useReactDev } from '@s4wave/web/devtools/react-dev.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'

// ReactDevStatusButton marks a document running Development React in the
// bottom bar, with the setting's controls in its popover.
export function ReactDevStatusButton() {
  const control = useReactDev()
  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) =>
      control && (
        <Popover open={selected}>
          <PopoverAnchor asChild>
            <BottomBarItem
              selected={selected}
              onClick={onClick}
              className={className}
              data-testid="react-dev-status-button"
            >
              <span className="text-warning font-mono text-xs">Dev React</span>
            </BottomBarItem>
          </PopoverAnchor>
          <PopoverContent
            side="top"
            align="end"
            sideOffset={6}
            onEscapeKeyDown={onClick}
            onPointerDownOutside={onClick}
            variant="status"
          >
            <div className="p-3">
              <ReactDevControls control={control} />
            </div>
          </PopoverContent>
        </Popover>
      ),
    [control],
  )

  if (!control?.model.running) return null
  return (
    <BottomBarLevel
      id="react-dev-status"
      position="right"
      button={buttonRender}
    >
      {null}
    </BottomBarLevel>
  )
}

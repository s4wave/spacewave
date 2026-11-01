import type { ReactNode } from 'react'
import { LuSquare } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import type { VmState } from '@s4wave/sdk/vm/v86.pb.js'

import { VmStateBadge } from './VmStateBadge.js'

// VmHeaderProps configures the header strip of a VM.
export interface VmHeaderProps {
  state: VmState
  // activity follows the state badge, such as the downloads of a run.
  activity?: ReactNode
  // children are the controls of a VM that is running or about to; a
  // stopped VM shows only its state.
  children?: ReactNode
}

// VmHeader is the 28px toolbar above a VM: its state and activity, then its
// controls. The tab names the VM, so the strip does not. The strip is a
// container, so controls can collapse their labels in a narrow panel, and it
// positions the activity's progress line along its bottom edge.
export function VmHeader({ state, activity, children }: VmHeaderProps) {
  return (
    <div className="border-foreground/8 @container relative flex h-7 shrink-0 items-center gap-1.5 border-b px-2">
      <div className="flex min-w-0 flex-1 items-center gap-1.5 select-none">
        <VmStateBadge state={state} />
        {activity}
      </div>
      {children}
    </div>
  )
}

// VmStopButtonProps configures the stop action of a VM.
export interface VmStopButtonProps {
  // disabled is set once a stop is requested.
  disabled: boolean
  onStop: () => void
}

// VmStopButton asks a running VM to stop. Its label collapses to the icon in
// a narrow header.
export function VmStopButton({ disabled, onStop }: VmStopButtonProps) {
  return (
    <Button
      variant="destructiveOutline"
      size="xs"
      aria-label="Stop"
      disabled={disabled}
      onClick={onStop}
    >
      <LuSquare />
      <span className="hidden @md:inline">Stop</span>
    </Button>
  )
}

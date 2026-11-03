import type { ReactNode } from 'react'
import { LuSquare } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import type { VmState } from '@s4wave/sdk/vm/v86.pb.js'

import { VmStateBadge } from './VmStateBadge.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    containerType: 'inline-size',
    display: 'flex',
    flexShrink: 0,
    position: 'relative',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule2: {
    WebkitUserSelect: 'none',
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'flex',
    userSelect: 'none',
    minWidth: 0,
  },
  rule3: {
    display: { default: 'none', '@container (min-width: 28rem)': 'inline' },
  },
})

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
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
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
      <span sx={stylexStyles.rule3}>Stop</span>
    </Button>
  )
}

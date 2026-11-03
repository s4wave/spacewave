import {
  LuKeyboard,
  LuMaximize,
  LuMonitor,
  LuTerminal,
} from '@s4wave/web/images/icons.js'

import { VmStopButton } from '@s4wave/app/vm/VmHeader.js'
import { VmViewSwitch } from '@s4wave/app/vm/VmViewSwitch.js'
import { Button } from '@s4wave/web/ui/button.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
  },
  rule2: {
    display: { default: 'none', '@container (min-width: 42rem)': 'inline' },
  },
})

// VmView is the panel the running view shows.
export type VmView = 'screen' | 'console'

// VmV86ControlsProps are the controls of a VM that is running or about to.
export interface VmV86ControlsProps {
  view: VmView
  onViewChange: (view: VmView) => void
  // stopDisabled is set once a stop is requested.
  stopDisabled: boolean
  onStop: () => void
  onCtrlAltDel: () => void
  onFullscreen: () => void
}

// viewTabs lists the panels of the running view.
const viewTabs = [
  { view: 'screen', label: 'Screen', icon: LuMonitor },
  { view: 'console', label: 'Console', icon: LuTerminal },
] as const

// VmV86Controls are the panel tabs and machine actions of a running x86 VM,
// shown in its header. Labels collapse to icons in a narrow panel.
export function VmV86Controls({
  view,
  onViewChange,
  stopDisabled,
  onStop,
  onCtrlAltDel,
  onFullscreen,
}: VmV86ControlsProps) {
  return (
    <>
      <VmViewSwitch
        options={viewTabs}
        view={view}
        onViewChange={onViewChange}
      />
      <div sx={stylexStyles.rule1}>
        <Button
          variant="ghost"
          size="xs"
          aria-label="Send Ctrl+Alt+Delete"
          title="Send Ctrl+Alt+Delete"
          onClick={onCtrlAltDel}
        >
          <LuKeyboard />
          <span sx={stylexStyles.rule2}>Ctrl+Alt+Del</span>
        </Button>
        <Button
          variant="ghost"
          size="xs"
          aria-label="Fullscreen"
          title="Fullscreen"
          onClick={onFullscreen}
        >
          <LuMaximize />
        </Button>
        <VmStopButton disabled={stopDisabled} onStop={onStop} />
      </div>
    </>
  )
}

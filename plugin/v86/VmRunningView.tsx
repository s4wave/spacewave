import { useRef, useState } from 'react'

import { VmErrorBanner } from '@s4wave/app/vm/VmErrorBanner.js'
import { VmHeader } from '@s4wave/app/vm/VmHeader.js'
import { VmState } from '@s4wave/sdk/vm/v86.pb.js'

import { postScreenInput } from './screen-channel.js'
import { VmScreen } from './VmScreen.js'
import { VmSerialConsole } from './VmSerialConsole.js'
import { VmV86Controls, type VmView } from './VmV86Controls.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { padding: `calc(${themeLayout['--spacing']} * 3)`, flexShrink: 0, },
  rule2: {
    flex: 1,
    backgroundColor: 'var(--color-black)',
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
  },
})

// VmRunningViewProps describes a VM that is running or about to.
export interface VmRunningViewProps {
  objectKey: string
  state: VmState
  // stopRequested is set once the VM's desired state is stopped.
  stopRequested: boolean
  onStop: () => void
  operationError: string
}

// VmRunningView shows a running VM: its screen and serial console, one at a
// time, under the header strip of machine actions. Both panels stay mounted
// so switching keeps the screen and the scrollback. It opens on the console,
// where the guest's shell is.
export function VmRunningView({
  objectKey,
  state,
  stopRequested,
  onStop,
  operationError,
}: VmRunningViewProps) {
  const [view, setView] = useState<VmView>('console')
  const panelRef = useRef<HTMLDivElement | null>(null)

  return (
    <>
      <VmHeader state={state}>
        <VmV86Controls
          view={view}
          onViewChange={setView}
          stopDisabled={stopRequested || state === VmState.VmState_STOPPING}
          onStop={onStop}
          onCtrlAltDel={() =>
            postScreenInput(objectKey, { kind: 'ctrl-alt-del' })
          }
          // The browser may refuse fullscreen; the panel then stays put.
          onFullscreen={() =>
            void panelRef.current?.requestFullscreen().catch(() => undefined)
          }
        />
      </VmHeader>
      {operationError && (
        <div sx={stylexStyles.rule1}>
          <VmErrorBanner title="Operation error" message={operationError} />
        </div>
      )}
      <div ref={panelRef} sx={stylexStyles.rule2}>
        <VmScreen objectKey={objectKey} hidden={view !== 'screen'} />
        <VmSerialConsole
          objectKey={objectKey}
          booting={state !== VmState.VmState_RUNNING}
          hidden={view !== 'console'}
        />
      </div>
    </>
  )
}

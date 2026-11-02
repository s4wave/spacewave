import { LuPlay, LuRotateCcw, LuTerminal } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { VmState } from '@s4wave/sdk/vm/v86.pb.js'

import { VmErrorBanner } from '@s4wave/app/vm/VmErrorBanner.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

// WasiVmMachineCardProps describes a stopped or failed WasiVM and its
// actions.
export interface WasiVmMachineCardProps {
  name: string
  state: VmState
  errorMessage: string
  // operationError is the failure of the last request from this view.
  operationError: string
  rootfsKey: string
  // init is the init command line, program first.
  init: string
  // startDisabled holds Start until the Space can bind the VM's process.
  startDisabled: boolean
  onStart: () => void
  onReset: () => void
}

// WasiVmMachineCard shows a WasiVM that is not running: what it runs and the
// action that starts it, or the error that stopped it and the reset.
export function WasiVmMachineCard({
  name,
  state,
  errorMessage,
  operationError,
  rootfsKey,
  init,
  startDisabled,
  onStart,
  onReset,
}: WasiVmMachineCardProps) {
  const failed = state === VmState.VmState_ERROR
  return (
    <div className="flex min-h-0 flex-1 justify-center overflow-y-auto p-4 sm:p-8">
      <section className="border-foreground/6 bg-background-card/30 flex h-fit w-full max-w-md flex-col gap-4 rounded-lg border p-4 backdrop-blur-sm">
        {/* Identity */}
        <div className="flex items-center gap-3">
          <div className="bg-foreground/5 text-foreground-alt flex size-10 shrink-0 items-center justify-center rounded-lg">
            <LuTerminal sx={stylexStyles.rule1} />
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="text-foreground truncate text-base font-medium">
              {name}
            </h2>
            <div className="text-foreground-alt text-xs">
              WebAssembly virtual machine
            </div>
          </div>
        </div>

        {failed && (
          <VmErrorBanner
            title="Runtime error"
            message={errorMessage || 'The VM stopped with an error.'}
          />
        )}
        {operationError && (
          <VmErrorBanner title="Operation error" message={operationError} />
        )}

        {/* Configuration */}
        <dl className="flex flex-col gap-1.5 text-xs">
          <div className="flex gap-4">
            <dt className="text-foreground-alt w-24 shrink-0">
              Root filesystem
            </dt>
            <dd
              className="text-foreground min-w-0 truncate font-mono"
              title={rootfsKey}
            >
              {rootfsKey || 'None'}
            </dd>
          </div>
          <div className="flex gap-4">
            <dt className="text-foreground-alt w-24 shrink-0">Init</dt>
            <dd className="text-foreground min-w-0 truncate font-mono">
              {init}
            </dd>
          </div>
        </dl>

        {/* Action */}
        {failed ? (
          <Button variant="destructiveOutline" onClick={onReset}>
            <LuRotateCcw />
            Reset
          </Button>
        ) : (
          <Button disabled={startDisabled} onClick={onStart}>
            <LuPlay />
            Start
          </Button>
        )}
      </section>
    </div>
  )
}

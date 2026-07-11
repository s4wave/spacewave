import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { PhaseChecklist } from './PhaseChecklist.js'

const pairingPhases = [
  { label: 'Connecting to your other device', done: false, active: true },
  { label: 'Establishing encrypted channel', done: false },
  { label: 'Preparing connection verification', done: false },
]

// PairingChannelProgress renders the channel setup state before pairing can
// show the verification phrase.
export function PairingChannelProgress() {
  return (
    <div className="border-foreground/10 bg-foreground/[0.02] space-y-4 rounded-lg border p-4">
      <div className="flex items-center gap-3">
        <div className="bg-brand/10 text-brand flex size-9 shrink-0 items-center justify-center rounded-lg">
          <Spinner className="text-brand" />
        </div>
        <div className="min-w-0">
          <h2 className="text-foreground text-sm font-medium">
            Establishing encrypted channel
          </h2>
          <p
            className="text-foreground-alt mt-0.5 text-xs leading-relaxed"
            aria-live="polite"
          >
            Setting up the connection with your other device…
          </p>
        </div>
      </div>

      <ProgressBar indeterminate />

      <PhaseChecklist phases={pairingPhases} className="px-0" />
    </div>
  )
}

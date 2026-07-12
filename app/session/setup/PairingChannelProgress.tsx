import { useEffect, useState } from 'react'
import { LuLink } from 'react-icons/lu'

import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'

import { PhaseChecklist } from './PhaseChecklist.js'

const pairingPhaseLabels = [
  'Connecting to your other device',
  'Establishing encrypted channel',
  'Preparing connection verification',
]
const PAIRING_STALL_TIMEOUT_MS = 10_000

export interface PairingChannelProgressProps {
  status?: PairingStatus
  stallTimeoutMs?: number
}

function getPairingProgress(status: PairingStatus) {
  switch (status) {
    case PairingStatus.PairingStatus_PEER_CONNECTED:
      return { completed: 1, active: 1 }
    case PairingStatus.PairingStatus_VERIFYING_EMOJI:
    case PairingStatus.PairingStatus_WAITING_FOR_REMOTE_CONFIRM:
      return { completed: 2, active: 2 }
    case PairingStatus.PairingStatus_VERIFIED:
    case PairingStatus.PairingStatus_BOTH_CONFIRMED:
      return { completed: pairingPhaseLabels.length, active: null }
    default:
      return { completed: 0, active: 0 }
  }
}

// PairingChannelProgress renders the current channel setup state from the
// pairing status stream before pairing can show the verification phrase.
export function PairingChannelProgress({
  status = PairingStatus.PairingStatus_WAITING_FOR_PEER,
  stallTimeoutMs = PAIRING_STALL_TIMEOUT_MS,
}: PairingChannelProgressProps) {
  const progress = getPairingProgress(status)
  const phases = pairingPhaseLabels.map((label, index) => ({
    label,
    done: index < progress.completed,
    active: index === progress.active,
  }))
  const activePhase =
    progress.active === null
      ? phases[phases.length - 1]
      : phases[progress.active]
  const progressValue = Math.round((progress.completed / phases.length) * 100)
  const [stalled, setStalled] = useState(false)

  useEffect(() => {
    setStalled(false)
    if (progress.active === null) return
    const timeout = window.setTimeout(() => setStalled(true), stallTimeoutMs)
    return () => window.clearTimeout(timeout)
  }, [progress.active, stallTimeoutMs, status])

  return (
    <div className="border-foreground/10 bg-foreground/[0.02] space-y-4 rounded-lg border p-4">
      <div className="flex items-center gap-3">
        <div className="bg-brand/10 text-brand flex size-9 shrink-0 items-center justify-center rounded-lg">
          <LuLink className="text-brand size-4" aria-hidden="true" />
        </div>
        <div className="min-w-0">
          <h2 className="text-foreground text-sm font-medium">
            {activePhase.label}
          </h2>
          <p
            className="text-foreground-alt mt-0.5 text-xs leading-relaxed"
            aria-live="polite"
          >
            Setting up the connection with your other device…
          </p>
          {stalled && (
            <p
              className="text-foreground-alt/70 mt-1 text-xs"
              aria-live="polite"
            >
              Still working… this is taking longer than usual
            </p>
          )}
        </div>
      </div>

      <div data-progress={progressValue}>
        <ProgressBar value={progressValue} />
      </div>

      <PhaseChecklist phases={phases} className="px-0" />
    </div>
  )
}

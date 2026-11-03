import { useEffect, useState } from 'react'
import { LuLink } from '@s4wave/web/images/icons.js'

import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'

import { PhaseChecklist } from './PhaseChecklist.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 9)`,
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: { minWidth: 0 },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule7: { paddingInline: 0 },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

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
    <div
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'border-foreground/10 bg-foreground/[0.02] space-y-4 rounded-lg border p-4'
      }
    >
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <LuLink sx={stylexStyles.rule1} aria-hidden="true" />
        </div>
        <div sx={stylexStyles2.rule3}>
          <h2 sx={stylexStyles2.rule4}>{activePhase.label}</h2>
          <p sx={stylexStyles2.rule5} aria-live="polite">
            Setting up the connection with your other device…
          </p>
          {stalled && (
            <p sx={stylexStyles2.rule6} aria-live="polite">
              Still working… this is taking longer than usual
            </p>
          )}
        </div>
      </div>

      <div data-progress={progressValue}>
        <ProgressBar value={progressValue} />
      </div>

      <PhaseChecklist phases={phases} {...stylex.props(stylexStyles2.rule7)} />
    </div>
  )
}

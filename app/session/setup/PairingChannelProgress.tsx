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

import { utilityState as themeUtilityState } from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 2%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule2: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    width: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: { minWidth: 0 },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule5: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule7: { paddingInline: 0 },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
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
  sx,
  status = PairingStatus.PairingStatus_WAITING_FOR_PEER,
  stallTimeoutMs = PAIRING_STALL_TIMEOUT_MS,
}: PairingChannelProgressProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
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
    <div sx={[stylexStyles3.rule1, sx]}>
      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule1]}>
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

      <div sx={nativeChildSpacing.s4} data-progress={progressValue}>
        <ProgressBar value={progressValue} />
      </div>

      <PhaseChecklist
        phases={phases}
        sx={[nativeChildSpacing.s4, stylexStyles2.rule7]}
      />
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
})

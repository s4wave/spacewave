import { useEffect, useEffectEvent, useState } from 'react'
import { LuCircleCheck, LuShieldCheck, LuX } from '@s4wave/web/images/icons.js'
import { useRetryWithAbort } from '@aptre/bldr-react'

import {
  PairingStatus,
  type WatchPairingStatusResponse,
} from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { AccountOutcome } from '@s4wave/core/pairing/pairing.pb.js'
import { pairingStatusIsTerminalFailure } from '@s4wave/app/loading/status/pairing.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { PairingChannelProgress } from './PairingChannelProgress.js'
import { PairingAccountChoice } from './PairingAccountChoice.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({ rule1: { textAlign: 'center' } })

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule4: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'center',
  },
  rule5: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 14)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    fontSize: 'var(--text-3xl)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
  },
  rule10: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    justifyContent: 'center',
  },
  rule11: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule12: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule14: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
  },
  rule15: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
})

interface PairingVerificationStepProps {
  session: Session | null | undefined
  onContinue: () => void
  onAbort: () => void
}

// usePairingStatus follows the session's pairing status until it reaches a
// terminal failure or both devices confirm. It returns the latest snapshot and
// the failure message, which setError also sets for local failures.
function usePairingStatus(session: Session | null | undefined) {
  const [snapshot, setSnapshot] = useState<WatchPairingStatusResponse>()
  const [error, setError] = useState<string>()

  useRetryWithAbort(
    async (signal) => {
      if (!session) return
      try {
        for await (const next of session.watchPairingStatus(signal)) {
          if (signal.aborted) return
          setSnapshot(next)
          if (pairingStatusIsTerminalFailure(next.status)) {
            setError(next.errorMessage || 'Pairing failed')
            return
          }
          if (next.status === PairingStatus.PairingStatus_BOTH_CONFIRMED) {
            return
          }
        }
      } catch (cause) {
        if (!signal.aborted) {
          setError(
            cause instanceof Error
              ? cause.message
              : 'Pairing connection closed',
          )
        }
      }
    },
    undefined,
    [session],
  )

  return { snapshot, error, setError }
}

// PairingVerificationStep presents the selected account and follows the provider's
// bilateral approval and enrollment operation across every pairing entry point.
export function PairingVerificationStep({
  sx,
  session,
  onContinue,
  onAbort,
}: PairingVerificationStepProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const { snapshot, error, setError } = usePairingStatus(session)
  const [submitted, setSubmitted] = useState(false)
  const continuePairing = useEffectEvent(onContinue)

  useEffect(() => {
    if (snapshot?.status === PairingStatus.PairingStatus_BOTH_CONFIRMED) {
      continuePairing()
    }
  }, [snapshot?.status])

  const confirm = async (approved: boolean) => {
    if (!session || submitted) return
    setSubmitted(true)
    try {
      await session.confirmSASMatch(approved)
      if (!approved) onAbort()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Confirmation failed')
      setSubmitted(false)
    }
  }

  if (
    snapshot?.status === PairingStatus.PairingStatus_SELECTING_ACCOUNT &&
    snapshot.choice &&
    !error
  ) {
    return (
      <PairingAccountChoice
        sx={sx}
        choice={snapshot.choice}
        receiving={snapshot.receiving ?? false}
        onChoose={async (outcome) => {
          if (!session) throw new Error('Pairing Session is unavailable')
          await session.selectPairingAccount(outcome)
        }}
        onAbort={() => void confirm(false)}
      />
    )
  }

  if (error) {
    return (
      <div sx={[stylexStyles3.rule1, sx]}>
        <h2 sx={[nativeChildSpacing.s4, stylexStyles2.rule1]}>
          Pairing failed
        </h2>
        <p sx={[nativeChildSpacing.s4, stylexStyles2.rule2]}>{error}</p>
        <button
          type="button"
          onClick={onAbort}
          sx={[nativeChildSpacing.s4, stylexStyles2.rule3]}
        >
          Try again
        </button>
      </div>
    )
  }

  return (
    <PairingVerification
      sx={sx}
      snapshot={snapshot}
      submitted={submitted}
      onConfirm={(approved) => void confirm(approved)}
    />
  )
}

// describePairing returns the heading, instructions, and account summary for a
// pairing snapshot. submitted marks that this device has already confirmed.
function describePairing(
  snapshot: WatchPairingStatusResponse | undefined,
  submitted: boolean,
) {
  const enrolling = snapshot?.status === PairingStatus.PairingStatus_ENROLLING
  const waiting =
    submitted ||
    snapshot?.status === PairingStatus.PairingStatus_WAITING_FOR_REMOTE_CONFIRM
  const accountName = snapshot?.accountName || 'this account'
  const remoteName = snapshot?.remoteLabel || 'the other device'
  const outcome = snapshot?.choice?.outcome
  const merging =
    outcome === AccountOutcome.AccountOutcome_MERGE_INTO_OFFERED ||
    outcome === AccountOutcome.AccountOutcome_MERGE_INTO_RECEIVING
  const source =
    outcome === AccountOutcome.AccountOutcome_MERGE_INTO_OFFERED
      ? snapshot?.choice?.receivingAccount
      : snapshot?.choice?.offeredAccount

  let summary = `Allow ${remoteName} to access ${accountName}. Other accounts stay separate.`
  if (merging) {
    summary = `Merge ${source?.displayName || 'the other account'} into ${accountName}. Move its Spaces and Sessions, keeping ${accountName}'s settings and storage provider.`
  } else if (snapshot?.receiving) {
    summary = `Add ${accountName} to this device. Other accounts stay separate.`
  }

  if (enrolling && merging) {
    return {
      busy: true,
      summary,
      heading: 'Merging accounts',
      body: 'Saving the account transition and transferring your Spaces. Source data stays recoverable until the transfer is complete.',
    }
  }
  if (enrolling) {
    return {
      busy: true,
      summary,
      heading: 'Connecting account',
      body: 'Saving account access and preparing your Spaces.',
    }
  }
  if (waiting) {
    return {
      busy: true,
      summary,
      heading: 'Waiting for other device',
      body: 'Confirm the same emoji on the other device.',
    }
  }
  return {
    busy: false,
    summary,
    heading: 'Verify connection',
    body: 'Confirm the emoji match on both devices to approve this account choice.',
  }
}

// PairingVerification shows the verification emoji and the confirm actions, or
// a spinner while enrollment or the other device is pending.
function PairingVerification({
  sx,
  snapshot,
  submitted,
  onConfirm,
}: {
  snapshot: WatchPairingStatusResponse | undefined
  submitted: boolean
  onConfirm: (approved: boolean) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const { busy, summary, heading, body } = describePairing(snapshot, submitted)
  const emoji = snapshot?.emoji ?? []

  return (
    <div sx={sx}>
      <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule4]}>
        <div sx={stylexStyles2.rule5}>
          <LuShieldCheck sx={stylexStyles.rule1} />
        </div>
        <h2 sx={stylexStyles2.rule1}>{heading}</h2>
        {snapshot?.accountId && <p sx={stylexStyles2.rule6}>{summary}</p>}
        <p sx={stylexStyles2.rule7}>{body}</p>
      </div>

      {emoji.length > 0 && (
        <div
          sx={[nativeChildSpacingNext.s4, stylexStyles2.rule8]}
          aria-label="Verification emoji"
        >
          {emoji.map((symbol, index) => (
            <div key={`${index}-${symbol}`} sx={stylexStyles2.rule9}>
              {symbol}
            </div>
          ))}
        </div>
      )}
      {busy ? (
        <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule10]}>
          <Spinner size="lg" />
        </div>
      ) : emoji.length === 0 ? (
        <PairingChannelProgress
          sx={nativeChildSpacingNext.s4}
          status={snapshot?.status}
        />
      ) : (
        <div sx={[nativeChildSpacingNext.s4, stylexStyles2.rule11]}>
          <button
            type="button"
            onClick={() => onConfirm(false)}
            sx={stylexStyles2.rule12}
          >
            <LuX sx={stylexStyles.rule2} />
            <span sx={stylexStyles2.rule13}>No, abort</span>
          </button>
          <button
            type="button"
            onClick={() => onConfirm(true)}
            sx={stylexStyles2.rule14}
          >
            <LuCircleCheck sx={stylexStyles.rule3} />
            <span sx={stylexStyles2.rule15}>Yes, they match</span>
          </button>
        </div>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

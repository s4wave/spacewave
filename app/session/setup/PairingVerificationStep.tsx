import { useEffect, useEffectEvent, useState } from 'react'
import { LuCircleCheck, LuShieldCheck, LuX } from 'react-icons/lu'
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
  session,
  onContinue,
  onAbort,
}: PairingVerificationStepProps) {
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
      <div className="space-y-4 text-center">
        <h2 className="text-foreground text-sm font-medium">Pairing failed</h2>
        <p className="text-destructive text-xs">{error}</p>
        <button
          type="button"
          onClick={onAbort}
          className="border-foreground/20 text-foreground h-10 w-full rounded-md border text-sm"
        >
          Try again
        </button>
      </div>
    )
  }

  return (
    <PairingVerification
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
  snapshot,
  submitted,
  onConfirm,
}: {
  snapshot: WatchPairingStatusResponse | undefined
  submitted: boolean
  onConfirm: (approved: boolean) => void
}) {
  const { busy, summary, heading, body } = describePairing(snapshot, submitted)
  const emoji = snapshot?.emoji ?? []

  return (
    <div className="space-y-4">
      <div className="flex flex-col items-center gap-2 text-center">
        <div className="bg-brand/10 flex size-10 items-center justify-center rounded-full">
          <LuShieldCheck className="text-brand size-5" />
        </div>
        <h2 className="text-foreground text-sm font-medium">{heading}</h2>
        {snapshot?.accountId && (
          <p className="text-foreground text-xs leading-relaxed">{summary}</p>
        )}
        <p className="text-foreground-alt text-xs leading-relaxed">{body}</p>
      </div>

      {emoji.length > 0 && (
        <div
          className="grid grid-cols-3 gap-2 px-4"
          aria-label="Verification emoji"
        >
          {emoji.map((symbol, index) => (
            <div
              key={`${index}-${symbol}`}
              className="bg-foreground/5 flex h-14 items-center justify-center rounded-md text-3xl"
            >
              {symbol}
            </div>
          ))}
        </div>
      )}
      {busy ? (
        <div className="flex h-10 justify-center">
          <Spinner size="lg" />
        </div>
      ) : emoji.length === 0 ? (
        <PairingChannelProgress status={snapshot?.status} />
      ) : (
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => onConfirm(false)}
            className="border-destructive/30 hover:bg-destructive/10 flex h-10 flex-1 items-center justify-center gap-2 rounded-md border"
          >
            <LuX className="text-destructive size-4" />
            <span className="text-destructive text-sm">No, abort</span>
          </button>
          <button
            type="button"
            onClick={() => onConfirm(true)}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 flex h-10 flex-1 items-center justify-center gap-2 rounded-md border"
          >
            <LuCircleCheck className="text-brand size-4" />
            <span className="text-foreground text-sm">Yes, they match</span>
          </button>
        </div>
      )}
    </div>
  )
}

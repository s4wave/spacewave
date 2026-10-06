import { useState } from 'react'

import {
  AccountOutcome,
  type AccountChoice,
  type AccountOffer,
} from '@s4wave/core/pairing/pairing.pb.js'
import { cn } from '@s4wave/web/style/utils.js'

interface PairingAccountChoiceProps {
  choice: AccountChoice
  receiving: boolean
  onChoose: (outcome: AccountOutcome) => Promise<void>
  onAbort: () => void
}

// accountLabel identifies the account using its name and provider type.
function accountLabel(account: AccountOffer | undefined): string {
  const provider =
    !account?.providerId || account.providerId === 'local' ? 'Local' : 'Cloud'
  return `${account?.displayName || 'Account'} (${provider})`
}

// PairingAccountChoice shows both accounts and lets the code-entering client
// propose the relationship that both clients must subsequently approve.
export function PairingAccountChoice({
  choice,
  receiving,
  onChoose,
  onAbort,
}: PairingAccountChoiceProps) {
  const [selected, setSelected] = useState(
    AccountOutcome.AccountOutcome_SIGN_IN_OFFERED,
  )
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()
  const offered = accountLabel(choice.offeredAccount)
  const current = accountLabel(choice.receivingAccount)
  const offeredMachine =
    choice.offeredAccount?.machineName || 'Device that shared the code'
  const receivingMachine =
    choice.receivingAccount?.machineName || 'Device entering the code'
  const inventory = (account: AccountOffer | undefined) => {
    const spaces = account?.spaceCount ?? 0
    const sessions = account?.sessionCount ?? 0
    return `${spaces} ${spaces === 1 ? 'Space' : 'Spaces'} and ${sessions} ${sessions === 1 ? 'Session' : 'Sessions'}`
  }
  const choices = [
    {
      outcome: AccountOutcome.AccountOutcome_SIGN_IN_OFFERED,
      label: `Sign in to ${offered}`,
      detail: `Add it on ${receivingMachine}. Keep ${current} separate.`,
    },
    {
      outcome: AccountOutcome.AccountOutcome_SIGN_IN_RECEIVING,
      label: `Sign in to ${current}`,
      detail: `Add it on ${offeredMachine}. Keep ${offered} separate.`,
    },
    {
      outcome: AccountOutcome.AccountOutcome_MERGE_INTO_OFFERED,
      label: `Merge into ${offered}`,
      detail: `Move ${inventory(choice.receivingAccount)} from ${current}. Keep ${offered}'s settings and storage provider.`,
    },
    {
      outcome: AccountOutcome.AccountOutcome_MERGE_INTO_RECEIVING,
      label: `Merge into ${current}`,
      detail: `Move ${inventory(choice.offeredAccount)} from ${offered}. Keep ${current}'s settings and storage provider.`,
    },
  ]

  const submit = async () => {
    setSubmitting(true)
    setError(undefined)
    try {
      await onChoose(selected)
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : 'Account selection failed',
      )
      setSubmitting(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2 text-center">
        <h2 className="text-foreground text-sm font-medium">
          Choose an account
        </h2>
        <p className="text-foreground-alt text-xs leading-relaxed">
          These devices use different accounts. Both devices will confirm the
          selected outcome before access changes.
        </p>
      </div>
      <dl className="bg-foreground/5 space-y-2 rounded-md p-3 text-xs">
        <div>
          <dt className="text-foreground-alt">{offeredMachine}</dt>
          <dd className="text-foreground font-medium break-words">{offered}</dd>
          <dd className="text-foreground-alt">
            {inventory(choice.offeredAccount)}
          </dd>
        </div>
        <div>
          <dt className="text-foreground-alt">{receivingMachine}</dt>
          <dd className="text-foreground font-medium break-words">{current}</dd>
          <dd className="text-foreground-alt">
            {inventory(choice.receivingAccount)}
          </dd>
        </div>
      </dl>
      {receiving ? (
        <>
          <fieldset disabled={submitting} className="space-y-2">
            <legend className="sr-only">Account outcome</legend>
            {choices.map(({ outcome, label, detail }) => (
              <label
                key={outcome}
                className={cn(
                  'flex cursor-pointer items-start gap-2 rounded-md border p-3',
                  selected === outcome
                    ? 'border-brand/50 bg-brand/10'
                    : 'border-foreground/20',
                )}
              >
                <input
                  type="radio"
                  name="pairing-account-outcome"
                  checked={selected === outcome}
                  onChange={() => setSelected(outcome)}
                  className="accent-brand mt-0.5"
                />
                <span className="min-w-0 space-y-1">
                  <span className="text-foreground block text-xs font-medium break-words">
                    {label}
                  </span>
                  <span className="text-foreground-alt block text-xs leading-relaxed">
                    {detail}
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
          {error && (
            <p role="alert" className="text-destructive text-xs">
              {error}
            </p>
          )}
          <button
            type="button"
            disabled={submitting}
            onClick={() => void submit()}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground h-10 w-full rounded-md border text-sm disabled:opacity-50"
          >
            {submitting ? 'Preparing account…' : 'Review and verify'}
          </button>
        </>
      ) : (
        <p role="status" className="text-foreground-alt text-center text-xs">
          Choose the outcome on the device entering the code, then review it
          here.
        </p>
      )}
      <button
        type="button"
        onClick={onAbort}
        className="text-foreground-alt hover:text-foreground h-8 w-full text-xs"
      >
        Cancel pairing
      </button>
    </div>
  )
}

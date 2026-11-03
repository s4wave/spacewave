import { useState } from 'react'

import {
  AccountOutcome,
  type AccountChoice,
  type AccountOffer,
} from '@s4wave/core/pairing/pairing.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { textAlign: 'center' },
  rule2: { minWidth: 0 },
})

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule3: { color: themeColors['--color-foreground-alt'] },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    overflowWrap: 'break-word',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule6: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    accentColor: themeColors['--color-brand'],
  },
  rule10: {
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    overflowWrap: 'break-word',
    color: themeColors['--color-foreground'],
  },
  rule11: {
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule12: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule13: {
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
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
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule14: {
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: '100%',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
  },
})

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
  sx,
  choice,
  receiving,
  onChoose,
  onAbort,
}: PairingAccountChoiceProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
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
    <div sx={sx}>
      <div sx={[nativeChildSpacingNextNext.s4, stylexStyles3.rule1]}>
        <h2 sx={[nativeChildSpacingNext.s2, stylexStyles.rule1]}>
          Choose an account
        </h2>
        <p sx={[nativeChildSpacingNext.s2, stylexStyles.rule2]}>
          These devices use different accounts. Both devices will confirm the
          selected outcome before access changes.
        </p>
      </div>
      <dl sx={[nativeChildSpacingNextNext.s4, stylexStyles2.rule1]}>
        <div sx={nativeChildSpacing.s2}>
          <dt sx={stylexStyles.rule3}>{offeredMachine}</dt>
          <dd sx={stylexStyles.rule4}>{offered}</dd>
          <dd sx={stylexStyles.rule3}>{inventory(choice.offeredAccount)}</dd>
        </div>
        <div sx={nativeChildSpacing.s2}>
          <dt sx={stylexStyles.rule3}>{receivingMachine}</dt>
          <dd sx={stylexStyles.rule4}>{current}</dd>
          <dd sx={stylexStyles.rule3}>{inventory(choice.receivingAccount)}</dd>
        </div>
      </dl>
      {receiving ? (
        <>
          <fieldset sx={nativeChildSpacingNextNext.s4} disabled={submitting}>
            <legend sx={[nativeChildSpacingNext.s2, stylexStyles.rule5]}>
              Account outcome
            </legend>
            {choices.map(({ outcome, label, detail }) => (
              <label
                key={outcome}
                sx={[
                  nativeChildSpacingNext.s2,
                  [
                    stylexStyles.rule6,
                    selected === outcome
                      ? stylexStyles.rule7
                      : stylexStyles.rule8,
                  ],
                ]}
              >
                <input
                  type="radio"
                  name="pairing-account-outcome"
                  checked={selected === outcome}
                  onChange={() => setSelected(outcome)}
                  sx={stylexStyles.rule9}
                />
                <span sx={stylexStyles3.rule2}>
                  <span sx={[nativeChildSpacingNext.s1, stylexStyles.rule10]}>
                    {label}
                  </span>
                  <span sx={[nativeChildSpacingNext.s1, stylexStyles.rule11]}>
                    {detail}
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
          {error && (
            <p
              role="alert"
              sx={[nativeChildSpacingNextNext.s4, stylexStyles.rule12]}
            >
              {error}
            </p>
          )}
          <button
            type="button"
            disabled={submitting}
            onClick={() => void submit()}
            sx={[nativeChildSpacingNextNext.s4, stylexStyles.rule13]}
          >
            {submitting ? 'Preparing account…' : 'Review and verify'}
          </button>
        </>
      ) : (
        <p
          role="status"
          sx={[nativeChildSpacingNextNext.s4, stylexStyles.rule14]}
        >
          Choose the outcome on the device entering the code, then review it
          here.
        </p>
      )}
      <button
        type="button"
        onClick={onAbort}
        sx={[nativeChildSpacingNextNext.s4, stylexStyles.rule15]}
      >
        Cancel pairing
      </button>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNextNext = stylex.create({
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

import { useCallback, useState } from 'react'
import { LuShield } from '@s4wave/web/images/icons.js'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import type { WatchAccountInfoResponse } from '@s4wave/sdk/account/account.pb.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import {
  AuthConfirmDialog,
  buildEntityCredential,
} from './AuthConfirmDialog.js'
import type { AuthCredential } from './AuthConfirmDialog.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useAccountDashboardState } from './AccountDashboardStateContext.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule7: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface SecurityLevelSectionProps {
  sx?: stylex.StyleXStyles
  className?: never
  account: Resource<Account>
  retainStepUp?: boolean
  // embedded hides the section heading and outer wrapper when rendered inside
  // a parent CollapsibleSection.
  embedded?: boolean
}

// SecurityLevelSection displays the current security level and threshold
// configuration when multiple auth methods are available.
export function SecurityLevelSection({
  sx,
  account,
  retainStepUp = false,
  embedded,
}: SecurityLevelSectionProps) {
  const state = useAccountDashboardState(account)
  if (state) {
    return (
      <SecurityLevelSectionContent
        sx={sx}
        account={account}
        retainStepUp={retainStepUp}
        embedded={embedded}
        accountInfoResource={state.accountInfo}
      />
    )
  }

  return (
    <SecurityLevelSectionWithWatch
      sx={sx}
      account={account}
      retainStepUp={retainStepUp}
      embedded={embedded}
    />
  )
}

function SecurityLevelSectionWithWatch(props: SecurityLevelSectionProps) {
  const accountInfoResource = useStreamingResource(
    props.account,
    (acc, signal) => acc.watchAccountInfo({}, signal),
    [],
  )

  return (
    <SecurityLevelSectionContent
      {...props}
      accountInfoResource={accountInfoResource}
    />
  )
}

interface SecurityLevelSectionContentProps extends SecurityLevelSectionProps {
  accountInfoResource: Resource<WatchAccountInfoResponse>
}

function SecurityLevelSectionContent({
  sx,
  account,
  retainStepUp = false,
  embedded,
  accountInfoResource,
}: SecurityLevelSectionContentProps) {
  const loading = accountInfoResource.loading
  const info = accountInfoResource.value

  const threshold = info?.authThreshold ?? 0
  const keypairCount = info?.keypairCount ?? 0
  const level = securityLevel(threshold, keypairCount)
  const [pendingThreshold, setPendingThreshold] = useState<number | null>(null)

  const handleSetThreshold = useCallback(
    async (credential: AuthCredential) => {
      if (pendingThreshold === null || !account.value) return
      await account.value.setSecurityLevel({
        threshold: pendingThreshold,
        credential: buildEntityCredential(credential),
      })
    },
    [account.value, pendingThreshold],
  )

  const content =
    !loading && info && keypairCount <= 1 ? null : (
      <>
        {loading && (
          <p sx={[stylexStyles2.rule1, embedded && sx]}>
            Loading security info…
          </p>
        )}
        {!loading && info && keypairCount > 1 && (
          <div sx={embedded && sx}>
            <div sx={[nativeChildSpacingNext.s3, stylexStyles2.rule2]}>
              <span sx={stylexStyles2.rule3}>{level.label}</span>
              <span sx={stylexStyles2.rule4}>
                {threshold + 1} of {keypairCount} required
              </span>
            </div>
            <p sx={[nativeChildSpacingNext.s3, stylexStyles2.rule5]}>
              {level.description}
            </p>

            <div sx={nativeChildSpacingNext.s3}>
              {securityLevels(keypairCount).map((opt) => (
                <RadioOption
                  sx={nativeChildSpacing.s1_5}
                  key={opt.value}
                  selected={opt.value === threshold}
                  onSelect={() => {
                    if (opt.value !== threshold) {
                      setPendingThreshold(opt.value)
                    }
                  }}
                  label={opt.label}
                  description={opt.description}
                />
              ))}
            </div>

            <p sx={[nativeChildSpacingNext.s3, stylexStyles2.rule6]}>
              Changing security level requires account re-authentication.
            </p>
          </div>
        )}
        <AuthConfirmDialog
          open={pendingThreshold !== null}
          onOpenChange={(open) => {
            if (!open) setPendingThreshold(null)
          }}
          title="Change security level"
          description="Confirm your identity to change the security threshold."
          confirmLabel="Change"
          intent={{
            kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_SET_SECURITY_LEVEL,
            title: 'Change security level',
            description:
              'Confirm your identity to change the security threshold.',
          }}
          onConfirm={handleSetThreshold}
          account={account}
          retainAfterClose={retainStepUp}
        />
      </>
    )

  if (!content) return null

  if (embedded) return content

  return (
    <section sx={sx}>
      <div sx={stylexStyles2.rule7}>
        <h2 sx={stylexStyles2.rule8}>
          <LuShield sx={stylexStyles.rule1} />
          Security Level
        </h2>
      </div>
      <InfoCard>{content}</InfoCard>
    </section>
  )
}

interface SecurityLevelInfo {
  label: string
  description: string
}

// securityLevel returns a human-readable label for a threshold value.
function securityLevel(threshold: number, count: number): SecurityLevelInfo {
  if (threshold === 0) {
    return {
      label: 'Standard',
      description: 'Any single auth method can authorize account changes.',
    }
  }
  if (count > 0 && threshold >= count - 1) {
    return {
      label: 'Maximum',
      description: 'All auth methods are required for account changes.',
    }
  }
  return {
    label: 'Enhanced',
    description: `${threshold + 1} auth methods required for account changes.`,
  }
}

interface SecurityLevelOption {
  value: number
  label: string
  description: string
}

// securityLevels returns available security level options for a given keypair count.
function securityLevels(count: number): SecurityLevelOption[] {
  const levels: SecurityLevelOption[] = [
    { value: 0, label: 'Standard', description: 'Any one method' },
  ]
  for (let i = 1; i < count - 1; i++) {
    levels.push({
      value: i,
      label: 'Enhanced',
      description: `${i + 1} methods required`,
    })
  }
  if (count >= 2) {
    levels.push({
      value: count - 1,
      label: 'Maximum',
      description: 'All methods required',
    })
  }
  return levels
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

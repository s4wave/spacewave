import { useCallback, useState } from 'react'
import { LuShield } from 'react-icons/lu'

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

export interface SecurityLevelSectionProps {
  account: Resource<Account>
  retainStepUp?: boolean
  // embedded hides the section heading and outer wrapper when rendered inside
  // a parent CollapsibleSection.
  embedded?: boolean
}

// SecurityLevelSection displays the current security level and threshold
// configuration when multiple auth methods are available.
export function SecurityLevelSection({
  account,
  retainStepUp = false,
  embedded,
}: SecurityLevelSectionProps) {
  const state = useAccountDashboardState(account)
  if (state) {
    return (
      <SecurityLevelSectionContent
        account={account}
        retainStepUp={retainStepUp}
        embedded={embedded}
        accountInfoResource={state.accountInfo}
      />
    )
  }

  return (
    <SecurityLevelSectionWithWatch
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
    !loading && info && keypairCount <= 1 ?
      null
    : <>
        {loading && (
          <p className="text-foreground-alt/40 text-xs">
            Loading security info...
          </p>
        )}
        {!loading && info && keypairCount > 1 && (
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-foreground text-xs font-medium">
                {level.label}
              </span>
              <span className="text-foreground-alt text-xs">
                {threshold + 1} of {keypairCount} required
              </span>
            </div>
            <p className="text-foreground-alt/50 text-xs">
              {level.description}
            </p>

            <div className="space-y-1.5">
              {securityLevels(keypairCount).map((opt) => (
                <RadioOption
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

            <p className="text-foreground-alt/60 text-xs">
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

  if (!content) return null

  if (embedded) return content

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuShield className="h-3.5 w-3.5" />
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

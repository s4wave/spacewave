import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowRight,
  LuCircleAlert,
  LuCircleCheck,
  LuKeyRound,
  LuRefreshCcw,
} from 'react-icons/lu'

import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { useStateAtom } from '@s4wave/web/state/persist.js'
import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import { SharedObjectSelfEnrollmentErrorCategory } from '@s4wave/sdk/session/shared-object-self-enrollment.pb.js'
import {
  AccountDashboardStateProvider,
  useAccountDashboardState,
} from '@s4wave/app/session/dashboard/AccountDashboardStateContext.js'
import { AuthConfirmDialog } from '@s4wave/app/session/dashboard/AuthConfirmDialog.js'
import { buildAccountEscalationStateResult } from '@s4wave/app/session/dashboard/useAccountEscalationState.js'
import {
  defaultSelfEnrollmentSkip,
  selfEnrollmentSkipAtomKey,
} from './self-enrollment-skip.js'

// SessionSelfEnrollmentInterstitial renders the post-sign-in self-enrollment gate.
export function SessionSelfEnrollmentInterstitial() {
  const session = SessionContext.useContext()
  const sessionInfo = useResource(
    session,
    (value, signal) => value.getSessionInfo(signal),
    [],
  )
  const providerRef = sessionInfo.value?.sessionRef?.providerResourceRef
  const account = useMountAccount(
    providerRef?.providerId ?? '',
    providerRef?.providerAccountId ?? '',
    providerRef?.providerId === 'spacewave',
  )
  return (
    <AccountDashboardStateProvider account={account}>
      <SessionSelfEnrollmentInterstitialContent account={account} />
    </AccountDashboardStateProvider>
  )
}

function SessionSelfEnrollmentInterstitialContent({
  account,
}: {
  account: ReturnType<typeof useMountAccount>
}) {
  const session = SessionContext.useContext()
  const onboarding = SpacewaveOnboardingContext.useContextSafe()?.onboarding
  const navigateSession = useSessionNavigate()
  const [unlockOpen, setUnlockOpen] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [, setSelfEnrollmentSkip] = useStateAtom(
    null,
    selfEnrollmentSkipAtomKey,
    defaultSelfEnrollmentSkip,
  )
  const enrollment = useResource(
    session,
    async (value, signal, cleanup) =>
      cleanup(await value.spacewave.mountSharedObjectSelfEnrollment(signal)),
    [],
  )
  const state = useStreamingResource(
    enrollment,
    (value, signal) => value.watchState(signal),
    [],
  )
  const failures = state.value?.failures ?? []
  const hasFailures = failures.length > 0
  const count = useMemo(
    () => state.value?.count ?? onboarding?.sessionSelfEnrollmentCount ?? 0,
    [onboarding?.sessionSelfEnrollmentCount, state.value?.count],
  )
  const completedCount = state.value?.completedSharedObjectIds?.length ?? 0
  const progress = count > 0 ? Math.min(100, (completedCount / count) * 100) : 0
  const isComplete =
    !!state.value &&
    !state.value.running &&
    count > 0 &&
    completedCount + failures.length >= count
  const generationKey =
    state.value?.generationKey || onboarding?.sessionSelfEnrollmentGenerationKey
  const escalationIntent = useMemo(
    () => ({
      kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_UNSPECIFIED,
      title: 'Unlock session access',
      description:
        'Unlock an account key to connect this session to your spaces.',
    }),
    [],
  )
  const accountState = useAccountDashboardState(account)
  const escalation =
    accountState ?
      buildAccountEscalationStateResult(
        escalationIntent,
        accountState.accountInfo.value?.authThreshold ?? 0,
        accountState.authMethods.value?.authMethods ?? [],
        accountState.entityKeypairs.value?.keypairs ?? [],
        accountState.entityKeypairs.value?.unlockedCount ?? 0,
        accountState.accountInfo.loading ||
          accountState.authMethods.loading ||
          accountState.entityKeypairs.loading,
      )
    : null
  const requiredSigners = escalation?.state.requirement?.requiredSigners ?? 1
  const unlockedSigners = escalation?.state.requirement?.unlockedSigners ?? 0
  const authReady =
    !!accountState && !escalation?.loading && unlockedSigners >= requiredSigners
  const showProgress = state.value?.running || (authReady && !isComplete)
  const progressKnown = !!state.value

  const handleUnlockConfirm = useCallback(async () => {
    setActionError(null)
    setUnlockOpen(false)
    try {
      await enrollment.value?.start()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }, [enrollment.value])
  const handleSkip = useCallback(async () => {
    if (!generationKey) return
    setActionError(null)
    try {
      await enrollment.value?.skip(generationKey)
      setSelfEnrollmentSkip({
        skippedKey: generationKey,
        skippedAt: Date.now(),
      })
      navigateSession({ path: '/', replace: true })
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }, [enrollment.value, generationKey, navigateSession, setSelfEnrollmentSkip])
  const handleContinue = useCallback(() => {
    navigateSession({ path: '/', replace: true })
  }, [navigateSession])
  const handleRetry = useCallback(async () => {
    setActionError(null)
    try {
      await enrollment.value?.start()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }, [enrollment.value])
  return (
    <div
      data-testid="self-enrollment-interstitial"
      className="bg-background-primary flex h-full w-full items-center justify-center overflow-auto px-4 py-6"
    >
      <div className="border-foreground/8 bg-background-card/30 w-full max-w-xl rounded-lg border p-4">
        <div className="flex items-start gap-3">
          <div className="bg-brand/10 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg">
            <LuKeyRound className="text-brand h-4.5 w-4.5" />
          </div>
          <div className="min-w-0 flex-1 space-y-3">
            <div>
              <h1 className="text-foreground text-sm font-semibold tracking-tight">
                Connect this session
              </h1>
              <p className="text-foreground-alt/60 mt-1 text-xs">
                {count === 1 ?
                  '1 space needs this session key.'
                : `${count} spaces need this session key.`}
              </p>
            </div>
            {showProgress ?
              <div className="space-y-2">
                <div className="text-foreground-alt/70 flex items-center justify-between text-xs">
                  <span>Connecting to {count} spaces</span>
                  <span>
                    {progressKnown ?
                      `${completedCount}/${count}`
                    : `${count} remaining`}
                  </span>
                </div>
                {progressKnown ?
                  <div className="bg-foreground/10 h-2 overflow-hidden rounded-full">
                    <div
                      className="bg-brand h-full rounded-full transition-[width]"
                      style={{ width: `${progress}%` }}
                    />
                  </div>
                : <div className="bg-foreground/10 relative h-2 overflow-hidden rounded-full">
                    <div className="bg-brand animate-progress-indeterminate absolute inset-y-0 w-1/3 rounded-full" />
                  </div>
                }
                {state.value?.currentSharedObjectId ?
                  <div className="text-foreground-alt/60 truncate text-xs">
                    {state.value.currentSharedObjectId}
                  </div>
                : null}
              </div>
            : isComplete ?
              <div className="text-foreground-alt/70 flex items-center gap-2 text-xs">
                {hasFailures ?
                  <LuCircleAlert className="text-warning h-3.5 w-3.5 shrink-0" />
                : <LuCircleCheck className="text-success h-3.5 w-3.5 shrink-0" />
                }
                <span>
                  {hasFailures ?
                    'Finished with spaces that still need attention.'
                  : 'All available spaces are connected.'}
                </span>
              </div>
            : null}
            {hasFailures ?
              <div className="space-y-2">
                {failures.map((failure) => (
                  <div
                    key={failure.sharedObjectId}
                    className="border-foreground/8 bg-background-card/50 rounded-md border p-2"
                  >
                    <div className="text-foreground flex items-center gap-2 text-xs font-medium">
                      <LuCircleAlert className="text-warning h-3.5 w-3.5 shrink-0" />
                      <span className="min-w-0 truncate">
                        {failure.sharedObjectId}
                      </span>
                    </div>
                    <p className="text-foreground-alt/60 mt-1 text-xs">
                      {failure.message ||
                        failureLabel(
                          failure.category ??
                            SharedObjectSelfEnrollmentErrorCategory.UNKNOWN,
                        )}
                    </p>
                    <div className="mt-2">
                      <DashboardButton
                        icon={<LuRefreshCcw className="h-3.5 w-3.5" />}
                        onClick={
                          (
                            failure.category ===
                            SharedObjectSelfEnrollmentErrorCategory.OPEN_OBJECT
                          ) ?
                            handleContinue
                          : handleRetry
                        }
                      >
                        {failureAction(
                          failure.category ??
                            SharedObjectSelfEnrollmentErrorCategory.UNKNOWN,
                        )}
                      </DashboardButton>
                    </div>
                  </div>
                ))}
              </div>
            : null}
            {actionError ?
              <p className="text-danger text-xs">{actionError}</p>
            : null}
            {!showProgress ?
              <div className="flex flex-wrap items-center gap-2">
                <DashboardButton
                  icon={<LuKeyRound className="h-3.5 w-3.5" />}
                  onClick={() => setUnlockOpen(true)}
                >
                  {isComplete ? 'Unlock and retry' : 'Unlock and continue'}
                </DashboardButton>
                <DashboardButton
                  icon={<LuArrowRight className="h-3.5 w-3.5" />}
                  onClick={isComplete ? handleContinue : handleSkip}
                >
                  {isComplete ? 'Continue to dashboard' : 'Skip for now'}
                </DashboardButton>
              </div>
            : null}
          </div>
        </div>
      </div>
      <AuthConfirmDialog
        open={unlockOpen}
        onOpenChange={setUnlockOpen}
        title="Unlock session access"
        description="Unlock an account key to connect this session to your spaces."
        confirmLabel="Unlock and continue"
        intent={escalationIntent}
        onConfirm={handleUnlockConfirm}
        account={account}
        retainAfterClose
      />
    </div>
  )
}

function failureLabel(category: SharedObjectSelfEnrollmentErrorCategory) {
  if (category === SharedObjectSelfEnrollmentErrorCategory.OPEN_OBJECT) {
    return 'Open the space to finish repair.'
  }
  if (category === SharedObjectSelfEnrollmentErrorCategory.REPORT) {
    return 'Report this issue.'
  }
  return 'Retry the connection.'
}

function failureAction(category: SharedObjectSelfEnrollmentErrorCategory) {
  if (category === SharedObjectSelfEnrollmentErrorCategory.OPEN_OBJECT) {
    return 'Open dashboard'
  }
  if (category === SharedObjectSelfEnrollmentErrorCategory.REPORT) {
    return 'Report'
  }
  return 'Retry now'
}

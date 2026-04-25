import { useCallback, useEffect, useRef, useState } from 'react'
import {
  LuArrowRight,
  LuCloud,
  LuMonitor,
  LuTriangleAlert,
} from 'react-icons/lu'
import superjson from 'superjson'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { PLAN_PRICE_MONTHLY } from '@s4wave/app/provider/spacewave/pricing.js'
import { useLocalSessionOnboardingContext } from '@s4wave/app/session/setup/LocalSessionOnboardingContext.js'
import {
  completeAndDismissLocalSessionOnboardingProviderChoice,
  isLocalSessionOnboardingComplete,
  localSessionOnboardingStoreId,
  parseLocalSessionOnboardingState,
  type LocalSessionOnboardingState,
} from '@s4wave/app/session/setup/local-session-onboarding-state.js'
import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import type { Root } from '@s4wave/sdk/root/root.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import { SetupPageLayout } from './SetupPageLayout.js'

// LocalSessionSetup bridges older local onboarding entry points into the full
// setup wizard. The cloud mode provisions or resumes a linked local session and
// then navigates into /setup. The local mode redirects stale /setup/free-local
// URLs into the full /setup flow.
export function LocalSessionSetup({
  mode,
  metadata,
}: {
  mode: 'cloud' | 'local'
  metadata?: SessionMetadata
}) {
  const navigate = useNavigate()

  if (mode === 'cloud') {
    if (!metadata) {
      return <PreparingLocalStorageScreen />
    }
    if (metadata.providerId === 'spacewave') {
      return <LocalSessionSetupCloudScreen navigate={navigate} />
    }
  }

  // Compute the path to the setup wizard relative to the current route.
  // Mode 'cloud' is at /plan/free, so ../../setup. Mode 'local' is at
  // /setup/free-local, so ../ (already inside /setup/).
  const setupPath = mode === 'cloud' ? '../../setup' : '../'

  return (
    <LocalSessionSetupRedirectScreen
      navigate={navigate}
      setupPath={setupPath}
    />
  )
}

function PreparingLocalStorageScreen() {
  return (
    <SetupPageLayout
      title="Preparing local storage"
      subtitle="Opening your full local setup flow."
    >
      <div className="flex flex-col items-center gap-2">
        <Spinner size="lg" className="text-foreground-alt" />
      </div>
    </SetupPageLayout>
  )
}

// LocalSessionSetupCloudScreen provisions or resumes the linked local session,
// then forwards into the full local setup wizard.
function LocalSessionSetupCloudScreen({
  navigate,
}: {
  navigate: (to: { path: string }) => void
}) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const rootResource = useRootResource()
  const [error, setError] = useState<string | null>(null)

  usePromise(
    useCallback(
      (signal: AbortSignal) => {
        const root = rootResource.value
        if (!session || !root) return undefined
        return (async () => {
          const resp = await session.spacewave.getLinkedLocalSession(signal)
          if (signal.aborted) return
          if (resp.found) {
            const localIdx = resp.sessionIndex ?? 0
            const state = await preparePlanReturnOnboardingForSessionIndex(
              root,
              localIdx,
              signal,
            )
            if (signal.aborted) return
            if (!shouldRouteLinkedLocalToSetup(state)) {
              navigate({ path: `/u/${localIdx}/` })
              return
            }
            navigate({ path: `/u/${localIdx}/setup` })
            return
          }
          const created = await session.spacewave.createLinkedLocalSession()
          if (signal.aborted) return
          const localIdx = created.sessionListEntry?.sessionIndex ?? 0
          await preparePlanReturnOnboardingForSessionIndex(
            root,
            localIdx,
            signal,
          )
          if (signal.aborted) return
          navigate({ path: `/u/${localIdx}/setup` })
        })().catch((err) => {
          if (signal.aborted) return
          setError(
            err instanceof Error ?
              err.message
            : 'Failed to prepare local session',
          )
        })
      },
      [session, rootResource.value, navigate],
    ),
  )

  if (error) {
    return (
      <SetupPageLayout
        title="Could not prepare local storage"
        subtitle="Please try again from the plan page."
      >
        <p className="text-destructive text-center text-xs">{error}</p>
      </SetupPageLayout>
    )
  }

  return <PreparingLocalStorageScreen />
}

// LocalSessionSetupRedirectScreen redirects stale local onboarding URLs into
// the full setup wizard so the user sees only one local setup screen.
function LocalSessionSetupRedirectScreen({
  navigate,
  setupPath,
}: {
  navigate: (to: { path: string }) => void
  setupPath: string
}) {
  const { loading, setOnboarding } = useLocalSessionOnboardingContext()
  const redirectedRef = useRef(false)

  useEffect(() => {
    if (loading) return
    if (redirectedRef.current) return
    redirectedRef.current = true
    setOnboarding(mergePlanReturnLocalSessionOnboarding)
    navigate({ path: setupPath })
  }, [loading, setOnboarding, navigate, setupPath])

  return <PreparingLocalStorageScreen />
}

function mergePlanReturnLocalSessionOnboarding(
  state: LocalSessionOnboardingState,
): LocalSessionOnboardingState {
  return completeAndDismissLocalSessionOnboardingProviderChoice(state)
}

function shouldRouteLinkedLocalToSetup(
  state: LocalSessionOnboardingState | null,
): boolean {
  if (!state) return true
  if (state.dismissed) return false
  return !isLocalSessionOnboardingComplete(
    completeAndDismissLocalSessionOnboardingProviderChoice(state, 0),
  )
}

async function preparePlanReturnOnboardingForSession(
  session: Session,
  signal: AbortSignal,
): Promise<LocalSessionOnboardingState> {
  using stateAtom = await session.accessStateAtom(
    { storeId: localSessionOnboardingStoreId },
    signal,
  )
  const resp = await stateAtom.getState(signal)
  const currentState = parseLocalSessionOnboardingState(resp.stateJson)
  const nextState = mergePlanReturnLocalSessionOnboarding(currentState)
  await stateAtom.setState(superjson.stringify(nextState), signal)
  return currentState
}

async function preparePlanReturnOnboardingForSessionIndex(
  root: Root,
  sessionIdx: number,
  signal: AbortSignal,
): Promise<LocalSessionOnboardingState | null> {
  if (sessionIdx <= 0) return null
  try {
    const mounted = await root.mountSessionByIdx({ sessionIdx }, signal)
    if (!mounted) return null
    using session = mounted.session
    return await preparePlanReturnOnboardingForSession(session, signal)
  } catch (err) {
    if (signal.aborted) return null
    console.warn(
      'failed to prepare local onboarding state for linked local session',
      err,
    )
  }
  return null
}

// WarningCard renders the browser storage warning with download and upgrade options.
export function WarningCard({
  onDownload,
  onUpgrade,
  upgradeLoading,
}: {
  onDownload: () => void
  onUpgrade: () => void
  upgradeLoading?: boolean
}) {
  return (
    <div className="border-foreground/20 bg-background-card/50 overflow-hidden rounded-lg border backdrop-blur-sm">
      <div className="p-6">
        <div className="mb-4 flex items-center gap-3">
          <div className="bg-brand/10 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
            <LuTriangleAlert className="text-brand h-5 w-5" />
          </div>
          <div>
            <h2 className="text-foreground text-sm font-medium">
              Backup your data
            </h2>
            <p className="text-foreground-alt mt-1 text-xs leading-relaxed">
              Your browser may clear stored data if disk space is low.
            </p>
          </div>
        </div>

        <div className="space-y-3">
          <button
            onClick={onDownload}
            className="border-foreground/10 hover:border-brand/30 hover:bg-brand/5 flex w-full cursor-pointer items-center gap-3 rounded-md border p-3 text-left transition-colors"
          >
            <div className="bg-foreground/5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
              <LuMonitor className="text-foreground-alt h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-foreground text-sm font-medium">
                Download the desktop app
              </p>
              <p className="text-foreground-alt mt-0.5 text-xs">
                Persistent storage on your computer.
              </p>
            </div>
            <LuArrowRight className="text-foreground-alt/50 h-4 w-4 shrink-0" />
          </button>

          <button
            onClick={onUpgrade}
            disabled={upgradeLoading}
            className="border-foreground/10 hover:border-brand/30 hover:bg-brand/5 flex w-full cursor-pointer items-center gap-3 rounded-md border p-3 text-left transition-colors disabled:opacity-50"
          >
            <div className="bg-foreground/5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md">
              {upgradeLoading ?
                <Spinner className="text-foreground-alt" />
              : <LuCloud className="text-foreground-alt h-4 w-4" />}
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-foreground text-sm font-medium">
                Upgrade to Cloud
              </p>
              <p className="text-foreground-alt mt-0.5 text-xs">
                Cloud sync and backup for ${PLAN_PRICE_MONTHLY}/mo.
              </p>
            </div>
            <LuArrowRight className="text-foreground-alt/50 h-4 w-4 shrink-0" />
          </button>
        </div>
      </div>
    </div>
  )
}

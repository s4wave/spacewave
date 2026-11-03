import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { isDesktop } from '@aptre/bldr'
import {
  LuArrowRight,
  LuCheck,
  LuCloud,
  LuMonitor,
  LuTriangleAlert,
} from '@s4wave/web/images/icons.js'
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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule2: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
  },
  rule4: { padding: `calc(${themeLayout['--spacing']} * 6)` },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule8: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule9: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    cursor: 'default',
    display: 'flex',
    opacity: 0.6,
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule10: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule11: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule12: { flex: 1, minWidth: 0 },
  rule13: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule14: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    opacity: { default: null, ':disabled': 0.5 },
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
      <div sx={stylexStyles2.rule1}>
        <Spinner size="lg" variant="muted" />
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
          // eslint-disable-next-line react-doctor/async-defer-await
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
          // eslint-disable-next-line react-doctor/async-defer-await
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
            err instanceof Error
              ? err.message
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
        <p sx={stylexStyles2.rule2}>{error}</p>
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
  const navigateEvent = useEffectEvent(navigate)

  useEffect(() => {
    if (loading) return
    if (redirectedRef.current) return
    redirectedRef.current = true
    setOnboarding(mergePlanReturnLocalSessionOnboarding)
    navigateEvent({ path: setupPath })
  }, [loading, setOnboarding, setupPath])

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
    <div sx={stylexStyles2.rule3}>
      <div sx={stylexStyles2.rule4}>
        <div sx={stylexStyles2.rule5}>
          <div sx={stylexStyles2.rule6}>
            <LuTriangleAlert sx={stylexStyles.rule1} />
          </div>
          <div>
            <h2 sx={stylexStyles2.rule7}>Backup your data</h2>
            <p sx={stylexStyles2.rule8}>
              Your browser may clear stored data if disk space is low.
            </p>
          </div>
        </div>

        <div>
          <button
            type="button"
            onClick={onDownload}
            disabled={isDesktop}
            sx={[
              nativeChildSpacing.s3,
              isDesktop ? stylexStyles2.rule9 : stylexStyles2.rule10,
            ]}
          >
            <div sx={stylexStyles2.rule11}>
              {isDesktop ? (
                <LuCheck sx={stylexStyles.rule2} />
              ) : (
                <LuMonitor sx={stylexStyles.rule2} />
              )}
            </div>
            <div sx={stylexStyles2.rule12}>
              <p sx={stylexStyles2.rule7}>Download the desktop app</p>
              <p sx={stylexStyles2.rule13}>
                Persistent storage on your computer.
              </p>
            </div>
            {!isDesktop && <LuArrowRight sx={stylexStyles.rule3} />}
          </button>

          <button
            type="button"
            onClick={onUpgrade}
            disabled={upgradeLoading}
            sx={[nativeChildSpacing.s3, stylexStyles2.rule14]}
          >
            <div sx={stylexStyles2.rule11}>
              {upgradeLoading ? (
                <Spinner variant="muted" />
              ) : (
                <LuCloud sx={stylexStyles.rule2} />
              )}
            </div>
            <div sx={stylexStyles2.rule12}>
              <p sx={stylexStyles2.rule7}>Upgrade to Cloud</p>
              <p sx={stylexStyles2.rule13}>
                Cloud sync and backup for ${PLAN_PRICE_MONTHLY}/mo.
              </p>
            </div>
            <LuArrowRight sx={stylexStyles.rule3} />
          </button>
        </div>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})

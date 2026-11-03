import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { LuFolderOpen, LuLink } from '@s4wave/web/images/icons.js'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { useAddSpaceRootAlias } from '@s4wave/app/hooks/useAddSpaceRootAlias.js'
import { findExistingSessionIndexByUsername } from '@s4wave/app/auth/find-existing-session.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { LoginForm } from '@s4wave/web/ui/login-form.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useSpacewaveAuth } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background-get-started'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    textAlign: 'center',
  },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    '--tw-duration': '.15s',
    transitionDuration: '.15s',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule5: { opacity: 0, pointerEvents: 'none' },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule7: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    pointerEvents: { default: null, ':disabled': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

interface AppLoginProps {
  initialUsername?: string
  launchUsername?: string
}

// AppLogin renders the login/signup screen with a unified password flow.
export function AppLogin({
  initialUsername,
  launchUsername,
}: AppLoginProps): React.ReactElement {
  const navigate = useNavigate()
  const [authBusy, setAuthBusy] = useState(false)
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const addRootAlias = useAddSpaceRootAlias()
  const normalizedLaunchUsername = useMemo(
    () => launchUsername?.trim().toLowerCase() ?? '',
    [launchUsername],
  )

  const existingSessionState = usePromise(
    useCallback(
      async (signal: AbortSignal) => {
        if (!root || !normalizedLaunchUsername) return undefined
        return await findExistingSessionIndexByUsername(
          root,
          normalizedLaunchUsername,
          signal,
        )
      },
      [normalizedLaunchUsername, root],
    ),
  )

  useEffect(() => {
    const sessionIndex = existingSessionState.data
    if (!sessionIndex) return
    navigate({ path: `/u/${sessionIndex}`, replace: true })
  }, [existingSessionState.data, navigate])

  const handleContinueWithoutAccount = useCallback(() => {
    navigate({ path: '/quickstart/local' })
  }, [navigate])

  const handleNavigateToSession = useCallback(
    (sessionIndex: number, _isNew: boolean) => {
      void (async () => {
        // If a session with the same providerAccountId is already mounted
        // at a different (earlier) index, redirect there to avoid duplicates.
        if (root) {
          const resp = await root.listSessions()
          const sessions = resp.sessions ?? []
          const target = sessions.find((s) => s.sessionIndex === sessionIndex)
          const accountId =
            target?.sessionRef?.providerResourceRef?.providerAccountId
          if (accountId) {
            for (const s of sessions) {
              const sid = s.sessionRef?.providerResourceRef?.providerAccountId
              if (
                sid === accountId &&
                s.sessionIndex != null &&
                s.sessionIndex !== sessionIndex
              ) {
                navigate({ path: `/u/${s.sessionIndex}` })
                return
              }
            }
          }
        }
        navigate({ path: `/u/${sessionIndex}` })
      })()
    },
    [navigate, root],
  )

  const auth = useSpacewaveAuth(handleNavigateToSession)

  const handleBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Escape') {
        handleBack()
      }
    },
    [handleBack],
  )

  const refCallback = useCallback((node: HTMLDivElement | null) => {
    if (node) {
      node.focus()
    }
  }, [])

  const checkingExistingSession =
    normalizedLaunchUsername.length !== 0 &&
    (root == null || existingSessionState.loading)
  if (checkingExistingSession || existingSessionState.data) {
    return (
      <AuthScreenLayout
        ref={refCallback}
        onKeyDown={handleKeyDown}
        tabIndex={-1}
        renderTopLeft={(buttonSx) => (
          <BackButton sx={buttonSx} onClick={handleBack}>
            Back to home
          </BackButton>
        )}
        intro={
          <>
            <AnimatedLogo followMouse={false} />
          </>
        }
      >
        <div sx={stylexStyles2.rule1}>
          <p sx={stylexStyles2.rule2}>Opening {normalizedLaunchUsername}...</p>
          <p sx={stylexStyles2.rule3}>
            If this account is already signed in here, Spacewave will open that
            session directly.
          </p>
        </div>
      </AuthScreenLayout>
    )
  }

  return (
    <AuthScreenLayout
      ref={refCallback}
      onKeyDown={handleKeyDown}
      tabIndex={-1}
      renderTopLeft={(buttonSx) => (
        <BackButton sx={buttonSx} onClick={handleBack}>
          Back to home
        </BackButton>
      )}
      renderTopRight={(buttonSx) => (
        <div sx={[stylexStyles2.rule4, !!authBusy && stylexStyles2.rule5]}>
          <button
            type="button"
            onClick={() => navigate({ path: '/pair' })}
            sx={[stylexStyles2.rule6, buttonSx, controlTargetStyles.minimum]}
          >
            <LuLink sx={stylexStyles.rule1} />
            <span sx={stylexStyles2.rule7}>Link to existing device</span>
          </button>
        </div>
      )}
      renderBottomRight={(buttonSx) => (
        <div sx={[stylexStyles2.rule4, !!authBusy && stylexStyles2.rule5]}>
          <button
            type="button"
            onClick={() => {
              void addRootAlias
                .add()
                .then((aliasId) => {
                  if (aliasId) navigate({ path: '/sessions' })
                })
                .catch(() => {})
            }}
            disabled={!addRootAlias.canAdd}
            sx={[stylexStyles2.rule8, buttonSx, controlTargetStyles.minimum]}
          >
            <LuFolderOpen sx={stylexStyles.rule1} />
            <span sx={stylexStyles2.rule7}>Open local state root</span>
          </button>
        </div>
      )}
      intro={
        <>
          <AnimatedLogo followMouse={false} />
        </>
      }
    >
      <LoginForm
        initialUsername={initialUsername ?? normalizedLaunchUsername}
        cloudProviderConfig={auth.cloudProviderConfig}
        onContinueWithoutAccount={handleContinueWithoutAccount}
        onLoginWithPassword={auth.handleLoginWithPassword}
        onCreateAccountWithPassword={auth.handleCreateAccountWithPassword}
        onLoginWithPem={auth.handleLoginWithPem}
        onNavigateToSession={handleNavigateToSession}
        onBrowserAuth={auth.handleContinueInBrowser}
        onContinueWithPasskey={auth.handleContinueWithPasskey}
        onSignInWithSSO={auth.handleSignInWithSSO}
        onAuthBusyChange={setAuthBusy}
      />
    </AuthScreenLayout>
  )
}

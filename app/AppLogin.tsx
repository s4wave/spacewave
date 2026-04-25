import React, { useCallback, useEffect, useMemo } from 'react'
import { LuLink } from 'react-icons/lu'

import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { findExistingSessionIndexByUsername } from '@s4wave/app/auth/find-existing-session.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { LoginForm } from '@s4wave/web/ui/login-form.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useSpacewaveAuth } from '@s4wave/app/provider/spacewave/useSpacewaveAuth.js'

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
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
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
        topLeft={<BackButton onClick={handleBack}>Back to home</BackButton>}
        intro={
          <>
            <AnimatedLogo followMouse={false} />
          </>
        }
      >
        <div className="border-foreground/20 bg-background-get-started rounded-lg border p-6 text-center shadow-lg">
          <p className="text-sm font-medium">
            Opening {normalizedLaunchUsername}...
          </p>
          <p className="text-foreground-alt mt-2 text-sm">
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
      topLeft={<BackButton onClick={handleBack}>Back to home</BackButton>}
      topRight={
        <button
          onClick={() => navigate({ path: '/pair' })}
          className="text-foreground-alt hover:text-brand flex items-center gap-1.5 text-xs transition-colors"
        >
          <LuLink className="h-3 w-3" />
          <span className="select-none">Link to existing device</span>
        </button>
      }
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
      />
    </AuthScreenLayout>
  )
}

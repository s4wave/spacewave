import { useCallback } from 'react'

import { useNavigate } from '@s4wave/web/router/router.js'
import { LoginForm } from '@s4wave/web/ui/login-form.js'
import { LuArrowLeft } from 'react-icons/lu'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { PageFooter } from './CloudConfirmationPage.js'
import { useSpacewaveAuth } from './useSpacewaveAuth.js'

// UpgradeLoginPage renders a login form for non-cloud sessions upgrading to cloud.
// After login or account creation, navigates to the new cloud session's /plan/upgrade
// to start the Stripe checkout flow.
export function UpgradeLoginPage() {
  const navigate = useNavigate()

  const navigateToSession = useCallback(
    (sessionIndex: number) => {
      navigate({ path: `/u/${sessionIndex}/plan/upgrade` })
    },
    [navigate],
  )

  const auth = useSpacewaveAuth(navigateToSession)

  const handleBack = useCallback(() => {
    navigate({ path: '../../' })
  }, [navigate])

  return (
    <AuthScreenLayout
      topLeft={
        <button
          onClick={handleBack}
          className="text-foreground-alt hover:text-brand flex items-center gap-2 text-sm transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          <span className="select-none">Back to plan selection</span>
        </button>
      }
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h1 className="mt-2 text-xl font-bold tracking-wide">
            Create a Cloud Account
          </h1>
        </>
      }
      contentClassName="auth-short:max-w-lg"
    >
      <div className="flex flex-col gap-6">
        <LoginForm
          cloudProviderConfig={auth.cloudProviderConfig}
          onLoginWithPassword={auth.handleLoginWithPassword}
          onCreateAccountWithPassword={auth.handleCreateAccountWithPassword}
          onNavigateToSession={navigateToSession}
          onBrowserAuth={auth.handleContinueInBrowser}
          onContinueWithPasskey={auth.handleContinueWithPasskey}
          onSignInWithSSO={auth.handleSignInWithSSO}
        />
        <PageFooter />
      </div>
    </AuthScreenLayout>
  )
}

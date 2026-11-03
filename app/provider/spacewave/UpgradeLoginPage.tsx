import { useCallback } from 'react'

import { useNavigate } from '@s4wave/web/router/router.js'
import { LoginForm } from '@s4wave/web/ui/login-form.js'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'
import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'
import { AuthScreenLayout } from '@s4wave/app/auth/AuthScreenLayout.js'
import { PageFooter } from './CloudConfirmationPage.js'
import { useSpacewaveAuth } from './useSpacewaveAuth.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule2: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule3: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule4: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

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
      renderTopLeft={(buttonSx) => (
        <button
          type="button"
          onClick={handleBack}
          sx={[stylexStyles2.rule1, buttonSx]}
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          <span sx={stylexStyles2.rule2}>Back to plan selection</span>
        </button>
      )}
      intro={
        <>
          <AnimatedLogo followMouse={false} />
          <h1 sx={stylexStyles2.rule3}>Create a Cloud Account</h1>
        </>
      }
      contentSx={authFormStyles.wide}
    >
      <div sx={stylexStyles2.rule4}>
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

// authFormStyles gives account creation its existing short-container width.
const authFormStyles = stylex.create({
  wide: {
    maxWidth: {
      default: 'var(--container-sm)',
      '@container (max-height: 740px)': 'var(--container-lg)',
    },
  },
})

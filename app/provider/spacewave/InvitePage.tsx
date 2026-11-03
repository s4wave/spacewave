import { useCallback, useState } from 'react'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOnboardingContext } from '@s4wave/web/contexts/SpacewaveOnboardingContext.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { Redirect } from '@s4wave/web/router/Redirect.js'
import { useNavigate, useParams } from '@s4wave/web/router/router.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { PageWrapper } from './CloudConfirmationPage.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: {
    marginInline: 'auto',
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
})

// InvitePage redeems the platform invite in the route for the session's
// account, then opens the session home. A sponsored invite stays on this page
// until the watched onboarding status shows the sponsored billing, so the home
// never routes the person to the plan page.
export function InvitePage() {
  const code = useParams().code ?? ''
  const session = useResourceValue(SessionContext.useContext())
  const onboarding = SpacewaveOnboardingContext.useContextSafe()
  const navigate = useNavigate()
  const [attempt, setAttempt] = useState(0)

  const redeem = usePromise(
    useCallback(
      (signal: AbortSignal) => session?.spacewave.redeemInvite(code, signal),
      [session, code, attempt], // eslint-disable-line react-hooks/exhaustive-deps -- attempt triggers a retry
    ),
  )
  const redeemed = redeem.data
  const retry = useCallback(() => setAttempt((n) => n + 1), [])
  const cancel = useCallback(
    () => navigate({ path: '../', replace: true }),
    [navigate],
  )

  if (redeemed && (!redeemed.sponsored || onboarding?.hasActiveBilling)) {
    return <Redirect to="../" />
  }
  return (
    <PageWrapper>
      <div sx={stylexStyles.rule1}>
        {redeem.error ? (
          <LoadingCard
            view={{
              state: 'error',
              title: 'This invite could not be redeemed',
              error: redeem.error.message,
              onRetry: retry,
              onCancel: cancel,
            }}
          />
        ) : (
          <LoadingCard
            view={{
              state: 'loading',
              title: redeemed
                ? 'Setting up your account'
                : 'Redeeming your invite',
              detail: redeemed
                ? 'Applying your sponsored plan.'
                : 'Checking the invite code.',
            }}
          />
        )}
      </div>
    </PageWrapper>
  )
}

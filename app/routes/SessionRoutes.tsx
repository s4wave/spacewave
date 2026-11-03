import { lazy } from 'react'
import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'

import { Route, useParams } from '@s4wave/web/router/router.js'
import { CheckoutResultPage } from '@s4wave/app/provider/spacewave/CheckoutResultPage.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { inviteRoute, joinRoute, storePendingRoute } from './pendingRoute.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule2: { width: '100%', maxWidth: 'var(--container-sm)' },
})

const LazyPairCodePage = lazy(async () => {
  const { PairCodePage } = await import('@s4wave/app/pair/PairCodePage.js')
  return { default: PairCodePage }
})

// SessionRedirect opens a session route in the first available session. With
// no session yet, it stores the route for SessionContainer to open after
// sign-in and sends the person to sign in.
function SessionRedirect({
  route,
  pending,
  title,
}: {
  route: string
  pending: boolean
  title: string
}) {
  const environment = useAppEnvironment()
  const resource = useSessionList()

  if (resource.loading) {
    return (
      <div sx={stylexStyles.rule1}>
        <div sx={stylexStyles.rule2}>
          <LoadingCard
            view={{
              state: 'loading',
              title,
              detail: 'Resolving your session before redirecting.',
            }}
          />
        </div>
      </div>
    )
  }

  const sessions = resource.value?.sessions ?? []
  if (sessions.length === 0) {
    if (pending) storePendingRoute(route, environment.documentStorage)
    return <NavigatePath to="/" replace />
  }

  const idx = sessions[0].sessionIndex ?? 1
  return <NavigatePath to={`/u/${idx}/${route}`} replace />
}

// JoinRedirect opens the join route of a Space invitation.
function JoinRedirect() {
  const code = useParams().code ?? ''
  return (
    <SessionRedirect
      route={code ? joinRoute(code) : 'join'}
      pending={!!code}
      title="Preparing join"
    />
  )
}

// InviteRedirect opens the redemption route of a platform invite.
function InviteRedirect() {
  const code = useParams().code ?? ''
  return (
    <SessionRedirect
      route={inviteRoute(code)}
      pending={true}
      title="Preparing your invite"
    />
  )
}

// SessionRoutes contains checkout, join, invite, and pairing entry routes.
// AppRoutes owns Quickstart and the mounted session surface.
export const SessionRoutes = (
  <>
    <Route path="/checkout/success">
      <CheckoutResultPage success />
    </Route>
    <Route path="/checkout/cancel">
      <CheckoutResultPage />
    </Route>
    <Route path="/join/:code">
      <JoinRedirect />
    </Route>
    <Route path="/join">
      <JoinRedirect />
    </Route>
    <Route path="/invite/:code">
      <InviteRedirect />
    </Route>
    <Route path="/pair/:code">
      <LazyPairCodePage />
    </Route>
    <Route path="/pair">
      <LazyPairCodePage />
    </Route>
  </>
)

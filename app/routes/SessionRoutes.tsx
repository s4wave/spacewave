import { Route, useParams } from '@s4wave/web/router/router.js'

import { CheckoutResultPage } from '@s4wave/app/provider/spacewave/CheckoutResultPage.js'
import { PairCodePage } from '@s4wave/app/pair/PairCodePage.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { AppQuickstart } from '../AppQuickstart.js'
import { storePendingJoin } from './pendingJoin.js'

export { consumePendingJoin, storePendingJoin } from './pendingJoin.js'

// JoinRedirect resolves the first available session and redirects to its join route.
function JoinRedirect() {
  const params = useParams()
  const code = params.code ?? ''
  const resource = useSessionList()

  if (resource.loading) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Preparing join',
              detail: 'Resolving your session before redirecting.',
            }}
          />
        </div>
      </div>
    )
  }

  const sessions = resource.value?.sessions ?? []
  if (sessions.length === 0) {
    // Stash the invite code so it survives account creation.
    if (code) storePendingJoin(code)
    return <NavigatePath to="/" replace />
  }

  const idx = sessions[0].sessionIndex ?? 1
  const target = code ? `/u/${idx}/join/${code}` : `/u/${idx}/join`
  return <NavigatePath to={target} replace />
}

// SessionRoutes contains routes for session entry: checkout, join, pair, and
// quickstart. The mounted session surface at /u/:sessionIndex/* is owned by
// AppRoutes (LazyAppSession) so the quickstart entry chunk stays free of the
// AppSession bundle until a session is actually opened.
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
    <Route path="/pair/:code">
      <PairCodePage />
    </Route>
    <Route path="/pair">
      <PairCodePage />
    </Route>
    <Route path="/quickstart/:quickstartId">
      <AppQuickstart />
    </Route>
  </>
)

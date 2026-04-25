import { Route, useParams } from '@s4wave/web/router/router.js'

import { CheckoutResultPage } from '@s4wave/app/provider/spacewave/CheckoutResultPage.js'
import { PairCodePage } from '@s4wave/app/pair/PairCodePage.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { AppQuickstart } from '../AppQuickstart.js'
import { AppSession } from '../AppSession.js'

// PENDING_JOIN_KEY is the sessionStorage key for a pending invite code.
const PENDING_JOIN_KEY = 'spacewave-pending-join'

// storePendingJoin saves an invite code to sessionStorage for post-setup pickup.
export function storePendingJoin(code: string) {
  if (code) sessionStorage.setItem(PENDING_JOIN_KEY, code)
}

// consumePendingJoin retrieves and clears a stored invite code.
export function consumePendingJoin(): string | null {
  const code = sessionStorage.getItem(PENDING_JOIN_KEY)
  if (code) sessionStorage.removeItem(PENDING_JOIN_KEY)
  return code
}

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

// SessionRoutes contains routes for sessions, quickstart, and checkout.
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
    <Route path="/u/:sessionIndex/*">
      <AppSession />
    </Route>
  </>
)

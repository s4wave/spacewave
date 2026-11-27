import { lazy, Suspense } from 'react'

import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { Route } from '@s4wave/web/router/router.js'

import { SESSION_PAGE_PREFIXES } from './session-page-path.js'

const LazySessionPages = lazy(async () => {
  const { SessionPages } = await import('./SessionPages.js')
  return { default: SessionPages }
})

// SessionPageRoutes keeps informational pages and their loading state in the
// session frame.
export const SessionPageRoutes = SESSION_PAGE_PREFIXES.map((prefix) => (
  <Route key={prefix} path={`/${prefix}/*`}>
    <SessionFrame>
      <Suspense fallback={null}>
        <LazySessionPages />
      </Suspense>
    </SessionFrame>
  </Route>
))

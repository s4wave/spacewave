import { useMemo } from 'react'

import { WebView } from '@aptre/bldr-react'

import { hasInteracted } from '@s4wave/web/state/interaction.js'
import { isStaticRoute } from '@s4wave/web/router/static-routes.js'
import { RouterProvider, type To } from '@s4wave/web/router/router.js'
import { AppLoadingScreen } from '@s4wave/app/loading/AppLoadingScreen.js'
import { PrerenderedApp } from './PrerenderedApp.js'
import { StaticProvider } from './StaticContext.js'
import { getStaticPageComponent } from './static-pages.js'

function handleStaticStartupNavigate(to: To) {
  const path = to.path
  if (!path) return

  if (isStaticRoute(path)) {
    window.location.href = path
    return
  }

  window.location.hash = path
}

// Startup renders the initial UI while the Go runtime loads.
// On static pages (pathname-based, no hash), renders the page
// component wrapped in StaticProvider to suppress RPC hooks.
// First-time visitors on non-static pages see the prerendered landing.
// Returning visitors see a loading screen.
export default function Startup() {
  const isStaticPage = useMemo(() => {
    return isStaticRoute(window.location.pathname) && !window.location.hash
  }, [])

  const loading = useMemo(() => {
    if (isStaticPage) {
      const PageComponent = getStaticPageComponent(window.location.pathname)
      if (PageComponent) {
        return (
          <RouterProvider
            path={window.location.pathname}
            onNavigate={handleStaticStartupNavigate}
          >
            <StaticProvider>
              <PageComponent />
            </StaticProvider>
          </RouterProvider>
        )
      }
    }
    if (hasInteracted()) return <AppLoadingScreen />
    return <PrerenderedApp />
  }, [isStaticPage])

  return <WebView loading={loading} />
}

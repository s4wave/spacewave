import { useMemo, type ComponentType } from 'react'

import { WebView } from '@aptre/bldr-react'

import { hasInteracted } from '@s4wave/web/state/interaction.js'
import { isPathnameAppRoute } from '@s4wave/web/router/app-path.js'
import { isStaticRoute } from '@s4wave/web/router/static-routes.js'
import { RouterProvider } from '@s4wave/web/router/router.js'
import { AppLoadingScreen } from '@s4wave/app/loading/AppLoadingScreen.js'
import { PrerenderedApp } from './PrerenderedApp.js'
import { StaticProvider } from './StaticContext.js'
import { navigateFromStaticStartup } from './static-navigate.js'
import { getStaticPageComponent } from './static-pages.js'

// shouldShowStartupLoading reports whether startup shows the loading screen
// instead of prerendered page content.
export function shouldShowStartupLoading(
  pathname: string,
  hash: string,
  interacted: boolean,
): boolean {
  if (hash.length > 1) return true
  if (interacted) return true
  return isPathnameAppRoute(pathname)
}

function StaticStartupPage({
  PageComponent,
}: {
  PageComponent: ComponentType
}) {
  return <PageComponent />
}

// Startup renders the initial UI while the Go runtime loads.
// On static pages (pathname-based, no hash), renders the page
// component wrapped in StaticProvider to suppress RPC hooks.
// App routes and returning visitors see a loading screen.
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
            onNavigate={navigateFromStaticStartup}
          >
            <StaticProvider>
              <StaticStartupPage PageComponent={PageComponent} />
            </StaticProvider>
          </RouterProvider>
        )
      }
    }
    if (
      shouldShowStartupLoading(
        window.location.pathname,
        window.location.hash,
        hasInteracted(),
      )
    ) {
      return <AppLoadingScreen />
    }
    return <PrerenderedApp />
  }, [isStaticPage])

  return <WebView loading={loading} startupProgress />
}

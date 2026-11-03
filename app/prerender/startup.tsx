import type { ReactNode } from 'react'

import { WebView } from '@aptre/bldr-react'

import { hasInteracted } from '@s4wave/web/state/interaction.js'
import { RouterProvider } from '@s4wave/web/router/router.js'
import { AppLoadingScreen } from '@s4wave/app/loading/AppLoadingScreen.js'
import { StaticProvider } from './StaticContext.js'
import { navigateFromStaticStartup } from './static-navigate.js'
import {
  selectStartupView,
  shouldShowStartupLoading,
  type StartupView,
} from './startup-view.js'

import '@s4wave/web/style/app.js'

// loadStartupView imports and renders only the components the view needs, so
// an app route does not fetch the public pages and their assets.
async function loadStartupView(
  view: StartupView,
  pathname: string,
  hash: string,
): Promise<ReactNode> {
  if (view === 'static') {
    const { getStaticPageComponent } = await import('./static-pages.js')
    const PageComponent = getStaticPageComponent(pathname)
    if (!PageComponent) {
      const loading = shouldShowStartupLoading(pathname, hash, hasInteracted())
      return loadStartupView(loading ? 'loading' : 'landing', pathname, hash)
    }
    return (
      <RouterProvider path={pathname} onNavigate={navigateFromStaticStartup}>
        <StaticProvider>
          <PageComponent />
        </StaticProvider>
      </RouterProvider>
    )
  }
  if (view === 'landing') {
    const { PrerenderedApp } = await import('./PrerenderedApp.js')
    return <PrerenderedApp />
  }
  return <AppLoadingScreen />
}

// The view is chosen once, when the startup module loads, and the module
// finishes loading only after the view's components have, so the first render
// shows the page as before.
const { pathname, hash } = window.location
const loading = await loadStartupView(
  selectStartupView(pathname, hash, hasInteracted()),
  pathname,
  hash,
)

// Startup renders the initial UI while the Go runtime loads.
// On static pages (pathname-based, no hash), renders the page
// component wrapped in StaticProvider to suppress RPC hooks.
// App routes and returning visitors see a loading screen.
export default function Startup() {
  return <WebView loading={loading} startupProgress />
}

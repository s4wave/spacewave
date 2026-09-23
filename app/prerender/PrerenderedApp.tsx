import { RouterProvider } from '@s4wave/web/router/router.js'
import { Landing } from '@s4wave/app/landing/Landing.js'

import { ROOT_LANDING_SHELL_CLASS } from './root-landing-shell.js'
import { StaticProvider } from './StaticContext.js'
import { navigateFromStaticStartup } from './static-navigate.js'

import '@s4wave/web/style/app.css'

// PrerenderedApp renders the root landing page the way the production
// prerender hydrates it: static mode, with page links loading their routes.
export function PrerenderedApp() {
  return (
    <RouterProvider path="/" onNavigate={navigateFromStaticStartup}>
      <StaticProvider>
        <div className={ROOT_LANDING_SHELL_CLASS}>
          <Landing />
        </div>
      </StaticProvider>
    </RouterProvider>
  )
}

export default PrerenderedApp

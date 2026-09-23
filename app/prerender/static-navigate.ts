import type { To } from '@s4wave/web/router/router.js'
import { isStaticRoute } from '@s4wave/web/router/static-routes.js'

// navigateFromStaticStartup routes a link from a startup-rendered static page.
// Static targets load their page like the production prerender; app targets
// move to the hash route the booting app reads.
export function navigateFromStaticStartup(to: To) {
  const path = to.path
  if (!path) return

  if (isStaticRoute(path)) {
    window.location.href = path
    return
  }

  window.location.hash = path
}

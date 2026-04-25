import { Route } from '@s4wave/web/router/router.js'

import { DocsIndexRoute } from '@s4wave/app/docs/DocsIndexRoute.js'
import { DocsSiteRoute } from '@s4wave/app/docs/DocsSiteRoute.js'
import { DocsPageRoute } from '@s4wave/app/docs/DocsPageRoute.js'

// DocsRoutes contains routes for documentation pages.
export const DocsRoutes = (
  <>
    <Route path="/docs">
      <DocsIndexRoute />
    </Route>
    <Route path="/docs/:site">
      <DocsSiteRoute />
    </Route>
    <Route path="/docs/:site/:section/:slug">
      <DocsPageRoute />
    </Route>
  </>
)

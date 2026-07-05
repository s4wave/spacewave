import { Route } from '@s4wave/web/router/router.js'

import { DisplayContainer } from './DisplayContainer.js'

// DisplayRoutes contains the top-level kiosk display route.
export const DisplayRoutes = (
  <Route path="/display/*">
    <DisplayContainer />
  </Route>
)

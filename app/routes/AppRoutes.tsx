import { Routes, Route } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'

import { LandingRoutes } from './LandingRoutes.js'
import { DocsRoutes } from './DocsRoutes.js'
import { BlogRoutes } from './BlogRoutes.js'
import { AuthRoutes } from './AuthRoutes.js'
import { SessionRoutes } from './SessionRoutes.js'
import { DebugRoutes } from './DebugRoutes.js'

// AppRoutes renders the appropriate content based on the current path.
export function AppRoutes() {
  return (
    <Routes fullPath>
      {LandingRoutes}
      {DocsRoutes}
      {BlogRoutes}
      {AuthRoutes}
      {SessionRoutes}
      {DebugRoutes}
      <Route path="*">
        <NavigatePath to="/" replace />
      </Route>
    </Routes>
  )
}

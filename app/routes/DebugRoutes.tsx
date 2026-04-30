import { Route } from '@s4wave/web/router/router.js'

import { CanvasGraphLinksDebug } from '@s4wave/web/debug/CanvasGraphLinksDebug.js'
import { DebugDbBench } from '@s4wave/web/debug/DebugDbBench.js'
import { ForgeViewerDebug } from '@s4wave/web/debug/ForgeViewerDebug.js'
import { HDRDebug } from '@s4wave/web/debug/HDRDebug.js'
import { LayoutDebug } from '@s4wave/web/debug/LayoutDebug.js'
import { LayoutColorsDebug } from '@s4wave/web/debug/LayoutColorsDebug.js'
import { LoadingDebug } from '@s4wave/web/debug/LoadingDebug.js'
import { SessionSettingsDebug } from '@s4wave/web/debug/SessionSettingsDebug.js'

// DebugRoutes contains routes for debug/development tools.
export const DebugRoutes = (
  <>
    <Route path="/debug/db/bench">
      <DebugDbBench />
    </Route>
    <Route path="/debug/hdr">
      <HDRDebug />
    </Route>
    <Route path="/debug/ui/layout">
      <LayoutDebug />
    </Route>
    <Route path="/debug/ui/layout/colors">
      <LayoutColorsDebug />
    </Route>
    <Route path="/debug/ui/canvas-graph-links">
      <CanvasGraphLinksDebug />
    </Route>
    <Route path="/debug/ui/session-settings">
      <SessionSettingsDebug />
    </Route>
    <Route path="/debug/ui/loading">
      <LoadingDebug />
    </Route>
    <Route path="/debug/ui/forge-viewer">
      <ForgeViewerDebug />
    </Route>
  </>
)

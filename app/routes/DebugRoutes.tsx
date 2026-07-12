import { Route } from '@s4wave/web/router/router.js'
import { KeybindingsDebug } from '@s4wave/app/debug/keybindings/KeybindingsDebug.js'
import { CanvasGraphLinksDebug } from '@s4wave/web/debug/CanvasGraphLinksDebug.js'
import { DebugDbBench } from '@s4wave/web/debug/DebugDbBench.js'
import { ForgeViewerDebug } from '@s4wave/web/debug/ForgeViewerDebug.js'
import { HDRDebug } from '@s4wave/web/debug/HDRDebug.js'
import { LayoutDebug } from '@s4wave/web/debug/LayoutDebug.js'
import { LayoutColorsDebug } from '@s4wave/web/debug/LayoutColorsDebug.js'
import { LoadingDebug } from '@s4wave/web/debug/LoadingDebug.js'
import { SessionSettingsDebug } from '@s4wave/web/debug/SessionSettingsDebug.js'
import { UnixFSBrowserDebug } from '@s4wave/web/debug/UnixFSBrowserDebug.js'

import { DebugIndex } from './DebugIndex.js'

const debugRouteEntries = [
  {
    path: '/debug/db/bench',
    title: 'DB bench',
    description: 'Run local database benchmark fixtures.',
    element: <DebugDbBench />,
  },
  {
    path: '/debug/hdr',
    title: 'HDR',
    description: 'Check high dynamic range color rendering.',
    element: <HDRDebug />,
  },
  {
    path: '/debug/ui/layout',
    title: 'Layout',
    description: 'Inspect the base layout and panel fixtures.',
    element: <LayoutDebug />,
  },
  {
    path: '/debug/ui/keybindings',
    title: 'Keyboard shortcuts',
    description: 'Compare interactive keybinding editor prototypes.',
    element: <KeybindingsDebug />,
  },
  {
    path: '/debug/ui/layout/colors',
    title: 'Layout colors',
    description: 'Review layout palette and contrast fixtures.',
    element: <LayoutColorsDebug />,
  },
  {
    path: '/debug/ui/canvas-graph-links',
    title: 'Canvas graph links',
    description: 'Exercise graph link rendering on canvas.',
    element: <CanvasGraphLinksDebug />,
  },
  {
    path: '/debug/ui/session-settings',
    title: 'Session settings',
    description: 'Inspect account and session settings fixtures.',
    element: <SessionSettingsDebug />,
  },
  {
    path: '/debug/ui/loading',
    title: 'Loading states',
    description: 'Review loading cards, progress, and spinners.',
    element: <LoadingDebug />,
  },
  {
    path: '/debug/ui/forge-viewer',
    title: 'Forge viewer',
    description: 'Inspect Forge task and job viewer fixtures.',
    element: <ForgeViewerDebug />,
  },
  {
    path: '/debug/ui/unixfs-browser',
    title: 'UnixFS browser',
    description: 'Exercise file browser fixtures.',
    element: <UnixFSBrowserDebug />,
  },
]

// DebugRoutes contains routes for debug/development tools.
export const DebugRoutes = (
  <>
    <Route path="/debug">
      <DebugIndex links={debugRouteEntries} />
    </Route>
    {debugRouteEntries.map((entry) => (
      <Route key={entry.path} path={entry.path}>
        {entry.element}
      </Route>
    ))}
  </>
)

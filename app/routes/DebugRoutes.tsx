import { Route } from '@s4wave/web/router/router.js'
import { GitSideDebug } from '@s4wave/app/debug/git/GitSideDebug.js'
import { ForgeViewerStatesDebug } from '@s4wave/app/debug/forge/ForgeViewerStatesDebug.js'
import { TerminalViewerDebug } from '@s4wave/app/debug/terminal/TerminalViewerDebug.js'
import { DeviceCapabilityDebug } from '@s4wave/app/debug/device/DeviceCapabilityDebug.js'
import { GalleryViewerDebug } from '@s4wave/app/debug/gallery/GalleryViewerDebug.js'
import { CanvasViewerDebug } from '@s4wave/app/debug/canvas/CanvasViewerDebug.js'
import { AccountRowsDebug } from '@s4wave/app/debug/account-rows/AccountRowsDebug.js'
import { KeybindingsDebug } from '@s4wave/app/debug/keybindings/KeybindingsDebug.js'
import { PromViewerDebug } from '@s4wave/app/debug/prom/PromViewerDebug.js'
import { WizardShellDebug } from '@s4wave/app/debug/wizard/WizardShellDebug.js'
import { TextFileStatesDebug } from '@s4wave/app/debug/unixfs/TextFileStatesDebug.js'
import { CanvasGraphLinksDebug } from '@s4wave/web/debug/CanvasGraphLinksDebug.js'
import { DebugDbBench } from '@s4wave/web/debug/DebugDbBench.js'
import { FactListDebug } from '@s4wave/web/debug/FactListDebug.js'
import { ViewerSideDebug } from '@s4wave/web/debug/ViewerSideDebug.js'
import { ViewerFloatDebug } from '@s4wave/web/debug/ViewerFloatDebug.js'
import { FileListDebug } from '@s4wave/web/debug/FileListDebug.js'
import { ViewerStatusDebug } from '@s4wave/web/debug/ViewerStatusDebug.js'
import { ForgeViewerDebug } from '@s4wave/web/debug/ForgeViewerDebug.js'
import { HDRDebug } from '@s4wave/web/debug/HDRDebug.js'
import { LayoutDebug } from '@s4wave/web/debug/LayoutDebug.js'
import { LayoutColorsDebug } from '@s4wave/web/debug/LayoutColorsDebug.js'
import { LoadingDebug } from '@s4wave/web/debug/LoadingDebug.js'
import { SessionSettingsDebug } from '@s4wave/web/debug/SessionSettingsDebug.js'
import { TableDebug } from '@s4wave/web/debug/TableDebug.js'
import { UnixFSBrowserDebug } from '@s4wave/web/debug/UnixFSBrowserDebug.js'

import { DebugIndex } from './DebugIndex.js'
import { appScenarios } from '../debug/scenarios/registry.js'
import { ScenarioPage } from '../debug/scenarios/ScenarioPage.js'

const debugRouteEntries = [
  ...appScenarios.map((scenario) => ({
    path: `/debug/ui/scenarios/${scenario.name}`,
    title: scenario.title,
    description: 'Open and reset a real app in an isolated temporary World.',
    element: <ScenarioPage scenario={scenario} />,
  })),
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
    path: '/debug/ui/account-rows',
    title: 'Account rows',
    description: 'Review the session picker account row in each state.',
    element: <AccountRowsDebug />,
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
    path: '/debug/ui/prom-viewer',
    title: 'Metrics viewers',
    description: 'Review Dashboard panel and Scrape target states.',
    element: <PromViewerDebug />,
  },
  {
    path: '/debug/ui/unixfs-browser',
    title: 'UnixFS browser',
    description: 'Exercise file browser fixtures.',
    element: <UnixFSBrowserDebug />,
  },
  {
    path: '/debug/ui/file-list',
    title: 'File list',
    description: 'Open the file list in one state; add /selected, /empty...',
    element: <FileListDebug />,
  },
  {
    path: '/debug/ui/text-file',
    title: 'Text file',
    description: 'Open a text file in one state; add /dirty, /changed...',
    element: <TextFileStatesDebug />,
  },
  {
    path: '/debug/ui/table',
    title: 'Table',
    description: 'Open the table in one state; add /selected, /by-latency...',
    element: <TableDebug />,
  },
  {
    path: '/debug/ui/facts',
    title: 'Facts',
    description: 'Open fact rows in one state; add /text, /two-fact.',
    element: <FactListDebug />,
  },
  {
    path: '/debug/ui/side',
    title: 'Side column',
    description: 'Open the side column in one state; add /rail, /details.',
    element: <ViewerSideDebug />,
  },
  {
    path: '/debug/ui/git-side',
    title: 'Git side column',
    description: 'Open the Git repository facts; add /rail, /details, /log...',
    element: <GitSideDebug />,
  },
  {
    path: '/debug/ui/forge-states',
    title: 'Forge viewer states',
    description: 'Open a Forge viewer in one state; add /log, /waiting...',
    element: <ForgeViewerStatesDebug />,
  },

  {
    path: '/debug/ui/terminal',
    title: 'Terminal',
    description: 'Open a terminal in one state; add /details, /watch-failed...',
    element: <TerminalViewerDebug />,
  },
  {
    path: '/debug/ui/device-capabilities',
    title: 'Device capabilities',
    description: 'Open the capability table; add /empty.',
    element: <DeviceCapabilityDebug />,
  },
  {
    path: '/debug/ui/gallery',
    title: 'Gallery',
    description: 'Open the image gallery in one state; add /again, /failed...',
    element: <GalleryViewerDebug />,
  },
  {
    path: '/debug/ui/canvas',
    title: 'Canvas',
    description: 'Open a canvas in one state; add /save-failed, /empty...',
    element: <CanvasViewerDebug />,
  },
  {
    path: '/debug/ui/viewer-status',
    title: 'Viewer status',
    description: 'Open a viewer waiting, failed or empty; add /failed, /empty.',
    element: <ViewerStatusDebug />,
  },
  {
    path: '/debug/ui/viewer-float',
    title: 'Viewer floats',
    description: 'Floating viewer controls; add /hidden, /pointer, /focused...',
    element: <ViewerFloatDebug />,
  },
  {
    path: '/debug/ui/wizard',
    title: 'Wizard',
    description: 'Open a wizard in one state; add /invalid, /final...',
    element: <WizardShellDebug />,
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
    <Route path="/debug/ui/file-list/:state">
      <FileListDebug />
    </Route>
    <Route path="/debug/ui/text-file/:state">
      <TextFileStatesDebug />
    </Route>
    <Route path="/debug/ui/table/:state">
      <TableDebug />
    </Route>
    <Route path="/debug/ui/facts/:state">
      <FactListDebug />
    </Route>
    <Route path="/debug/ui/side/:state">
      <ViewerSideDebug />
    </Route>
    <Route path="/debug/ui/git-side/:state">
      <GitSideDebug />
    </Route>
    <Route path="/debug/ui/forge-states/:state">
      <ForgeViewerStatesDebug />
    </Route>
    <Route path="/debug/ui/terminal/:state">
      <TerminalViewerDebug />
    </Route>
    <Route path="/debug/ui/device-capabilities/:state">
      <DeviceCapabilityDebug />
    </Route>
    <Route path="/debug/ui/gallery/:state">
      <GalleryViewerDebug />
    </Route>
    <Route path="/debug/ui/canvas/:state">
      <CanvasViewerDebug />
    </Route>
    <Route path="/debug/ui/viewer-status/:state">
      <ViewerStatusDebug />
    </Route>
    <Route path="/debug/ui/viewer-float/:state">
      <ViewerFloatDebug />
    </Route>
    <Route path="/debug/ui/wizard/:state">
      <WizardShellDebug />
    </Route>
  </>
)

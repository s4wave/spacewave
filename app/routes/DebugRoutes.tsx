import { lazy, type ComponentType, type LazyExoticComponent } from 'react'

import { Route } from '@s4wave/web/router/router.js'

import { DebugIndex } from './DebugIndex.js'
import { appScenarios } from '../debug/scenarios/registry.js'
import { ScenarioPage } from '../debug/scenarios/ScenarioPage.js'

// debugPage returns a component that loads the debug page named name from its
// module when it first renders, so the debug index loads no page. The route
// table renders inside AppRoutes' Suspense boundary.
function debugPage<K extends string>(
  load: () => Promise<Record<K, ComponentType>>,
  name: K,
): LazyExoticComponent<ComponentType> {
  return lazy<ComponentType>(async () => ({ default: (await load())[name] }))
}

const GitSideDebug = debugPage(
  () => import('@s4wave/app/debug/git/GitSideDebug.js'),
  'GitSideDebug',
)
const ForgeViewerStatesDebug = debugPage(
  () => import('@s4wave/app/debug/forge/ForgeViewerStatesDebug.js'),
  'ForgeViewerStatesDebug',
)
const TerminalViewerDebug = debugPage(
  () => import('@s4wave/app/debug/terminal/TerminalViewerDebug.js'),
  'TerminalViewerDebug',
)
const DeviceCapabilityDebug = debugPage(
  () => import('@s4wave/app/debug/device/DeviceCapabilityDebug.js'),
  'DeviceCapabilityDebug',
)
const GalleryViewerDebug = debugPage(
  () => import('@s4wave/app/debug/gallery/GalleryViewerDebug.js'),
  'GalleryViewerDebug',
)
const CanvasViewerDebug = debugPage(
  () => import('@s4wave/app/debug/canvas/CanvasViewerDebug.js'),
  'CanvasViewerDebug',
)
const AccountRowsDebug = debugPage(
  () => import('@s4wave/app/debug/account-rows/AccountRowsDebug.js'),
  'AccountRowsDebug',
)
const KeybindingsDebug = debugPage(
  () => import('@s4wave/app/debug/keybindings/KeybindingsDebug.js'),
  'KeybindingsDebug',
)
const PromViewerDebug = debugPage(
  () => import('@s4wave/app/debug/prom/PromViewerDebug.js'),
  'PromViewerDebug',
)
const WizardShellDebug = debugPage(
  () => import('@s4wave/app/debug/wizard/WizardShellDebug.js'),
  'WizardShellDebug',
)
const TextFileStatesDebug = debugPage(
  () => import('@s4wave/app/debug/unixfs/TextFileStatesDebug.js'),
  'TextFileStatesDebug',
)
const CanvasGraphLinksDebug = debugPage(
  () => import('@s4wave/web/debug/CanvasGraphLinksDebug.js'),
  'CanvasGraphLinksDebug',
)
const DebugDbBench = debugPage(
  () => import('@s4wave/web/debug/DebugDbBench.js'),
  'DebugDbBench',
)
const FactListDebug = debugPage(
  () => import('@s4wave/web/debug/FactListDebug.js'),
  'FactListDebug',
)
const ViewerSideDebug = debugPage(
  () => import('@s4wave/web/debug/ViewerSideDebug.js'),
  'ViewerSideDebug',
)
const ViewerFloatDebug = debugPage(
  () => import('@s4wave/web/debug/ViewerFloatDebug.js'),
  'ViewerFloatDebug',
)
const FileListDebug = debugPage(
  () => import('@s4wave/web/debug/FileListDebug.js'),
  'FileListDebug',
)
const ViewerStatusDebug = debugPage(
  () => import('@s4wave/web/debug/ViewerStatusDebug.js'),
  'ViewerStatusDebug',
)
const ForgeViewerDebug = debugPage(
  () => import('@s4wave/web/debug/ForgeViewerDebug.js'),
  'ForgeViewerDebug',
)
const HDRDebug = debugPage(
  () => import('@s4wave/web/debug/HDRDebug.js'),
  'HDRDebug',
)
const LayoutDebug = debugPage(
  () => import('@s4wave/web/debug/LayoutDebug.js'),
  'LayoutDebug',
)
const LayoutColorsDebug = debugPage(
  () => import('@s4wave/web/debug/LayoutColorsDebug.js'),
  'LayoutColorsDebug',
)
const LoadingDebug = debugPage(
  () => import('@s4wave/web/debug/LoadingDebug.js'),
  'LoadingDebug',
)
const SessionSettingsDebug = debugPage(
  () => import('@s4wave/web/debug/SessionSettingsDebug.js'),
  'SessionSettingsDebug',
)
const TableDebug = debugPage(
  () => import('@s4wave/web/debug/TableDebug.js'),
  'TableDebug',
)
const UnixFSBrowserDebug = debugPage(
  () => import('@s4wave/web/debug/UnixFSBrowserDebug.js'),
  'UnixFSBrowserDebug',
)

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

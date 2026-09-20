import { lazy } from 'react'

import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'

/** baseObjectViewers registers shared metadata without loading viewer implementations. */
export const baseObjectViewers: ObjectViewerComponent[] = [
  {
    componentID: 'spacewave.object-layout.viewer',
    typeID: 'alpha/object-layout',
    name: 'Layout Viewer',
    category: 'Layout',
    component: lazy(() =>
      import('@s4wave/web/object/LayoutObjectViewer.js').then((module) => ({
        default: module.LayoutObjectViewer,
      })),
    ),
  },
  {
    componentID: 'spacewave.debug.viewer',
    typeID: '*',
    name: 'Debug Viewer',
    category: 'Developer',
    component: lazy(() =>
      import('@s4wave/web/object/DebugObjectViewer.js').then((module) => ({
        default: module.DebugObjectViewer,
      })),
    ),
  },
]

/** getBaseObjectViewers returns shared viewers in default selection order. */
export function getBaseObjectViewers(): ObjectViewerComponent[] {
  return [...baseObjectViewers]
}

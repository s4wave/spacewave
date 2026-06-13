import { DebugObjectViewer } from '@s4wave/web/object/DebugObjectViewer.js'
import {
  LayoutObjectViewer,
  ObjectLayoutTypeID,
} from '@s4wave/web/object/LayoutObjectViewer.js'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'

export const baseObjectViewers: ObjectViewerComponent[] = [
  {
    componentID: 'spacewave.object-layout.viewer',
    typeID: ObjectLayoutTypeID,
    name: 'Layout Viewer',
    category: 'Layout',
    component: LayoutObjectViewer,
  },
  {
    componentID: 'spacewave.debug.viewer',
    typeID: '*',
    name: 'Debug Viewer',
    category: 'Developer',
    component: DebugObjectViewer,
  },
]

export function getBaseObjectViewers(): ObjectViewerComponent[] {
  return [...baseObjectViewers]
}

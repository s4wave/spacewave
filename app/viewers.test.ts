import { describe, expect, it } from 'vitest'

import { DriveTypeID } from '@s4wave/sdk/space/drive/drive.js'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'

import { getObjectViewersForType } from './viewers.js'

describe('getObjectViewersForType', () => {
  it('registers Drive as the primary file app surface', () => {
    const viewers = getObjectViewersForType(DriveTypeID)

    expect(viewers[0]?.name).toBe('Drive')
  })

  it('keeps the UnixFS browser ahead of the gallery in default order', () => {
    const viewers = getObjectViewersForType('unixfs/fs-node')

    expect(viewers.slice(0, 2).map((viewer) => viewer.name)).toEqual([
      'UnixFS Viewer',
      'UnixFS Gallery',
    ])
  })

  it('lets dynamic exact viewers passively win over wildcard fallback', () => {
    function DynamicViewer() {
      return null
    }
    const dynamicViewers: ObjectViewerComponent[] = [
      {
        typeID: 'plugin/custom',
        name: 'Plugin Surface',
        component: DynamicViewer,
      },
    ]

    const viewers = getObjectViewersForType('plugin/custom', dynamicViewers)

    expect(viewers.map((viewer) => viewer.name)).toEqual([
      'Plugin Surface',
      'Debug Viewer',
    ])
  })
})

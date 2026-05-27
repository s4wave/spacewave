import { describe, expect, it } from 'vitest'

import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import {
  getViewersForType,
  viewerRegistrationToComponent,
} from './useViewerRegistry.js'

function component(
  componentID: string,
  typeID: string,
  name = componentID,
): ObjectViewerComponent {
  return {
    componentID,
    typeID,
    name,
    component: () => null,
  }
}

describe('getViewersForType', () => {
  it('orders exact, prefix, then wildcard viewer registrations by component ID owner', () => {
    const viewers = [
      component('spacewave.debug.viewer', '*', 'Debug'),
      component('glados.generic.viewer', 'glados/*', 'GLaDOS Generic'),
      component('glados.workfront.viewer', 'glados/workfront', 'Workfront'),
    ]

    expect(
      getViewersForType('glados/workfront', viewers).map(
        (viewer) => viewer.componentID,
      ),
    ).toEqual([
      'glados.workfront.viewer',
      'glados.generic.viewer',
      'spacewave.debug.viewer',
    ])
  })
})

describe('viewerRegistrationToComponent', () => {
  it('maps dynamic registrations with stable component IDs and display names', () => {
    const viewer = viewerRegistrationToComponent({
      componentId: 'glados.workfront.viewer',
      typeId: 'glados/workfront',
      viewerName: 'Workfront',
      scriptPath: '/plugins/glados/workfront.js',
      category: 'GLaDOS',
    })

    expect(viewer).toMatchObject({
      componentID: 'glados.workfront.viewer',
      typeID: 'glados/workfront',
      name: 'Workfront',
      category: 'GLaDOS',
    })
  })

  it('rejects dynamic registrations without component IDs', () => {
    expect(
      viewerRegistrationToComponent({
        typeId: 'glados/workfront',
        viewerName: 'Workfront',
        scriptPath: '/plugins/glados/workfront.js',
      }),
    ).toBeNull()
  })
})

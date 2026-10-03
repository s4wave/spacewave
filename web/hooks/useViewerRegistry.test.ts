import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'
import type { Root } from '@s4wave/sdk/root'
import {
  ViewerSurface,
  type ViewerRegistration,
} from '@s4wave/sdk/viewer/registry/registry.pb.js'

const h = vi.hoisted(() => ({
  useDynamicRegistrations: vi.fn(
    (..._args: unknown[]): ObjectViewerComponent[] => [],
  ),
}))

vi.mock('./useDynamicRegistrations.js', () => ({
  useDynamicRegistrations: h.useDynamicRegistrations,
}))

import {
  getViewersForType,
  useAllViewers,
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
      component('gizmo.generic.viewer', 'gizmo/*', 'Gizmo Generic'),
      component('gizmo.worklist.viewer', 'gizmo/worklist', 'Worklist'),
    ]

    expect(
      getViewersForType('gizmo/worklist', viewers).map(
        (viewer) => viewer.componentID,
      ),
    ).toEqual([
      'gizmo.worklist.viewer',
      'gizmo.generic.viewer',
      'spacewave.debug.viewer',
    ])
  })
})

describe('useAllViewers', () => {
  it('does not pass terminal registrations to dynamic viewer conversion', () => {
    const webRegistration: ViewerRegistration = {
      componentId: 'gizmo.worklist.viewer',
      typeId: 'gizmo/worklist',
      viewerName: 'Worklist',
      scriptPath: '/plugins/gizmo/worklist.js',
      surface: ViewerSurface.WEB,
    }
    const terminalRegistration: ViewerRegistration = {
      ...webRegistration,
      componentId: 'terminal.worklist.viewer',
      surface: ViewerSurface.TUI,
    }
    const mappedRegistrations: ViewerRegistration[] = []

    h.useDynamicRegistrations.mockImplementationOnce((...args: unknown[]) => {
      const request = args[2] as { surface?: ViewerSurface }
      expect(request).toEqual({
        surface: ViewerSurface.WEB,
        instanceKey: 'space/one',
      })
      const mapper = args[6] as (
        registration: ViewerRegistration,
      ) => ObjectViewerComponent | null
      return [webRegistration, terminalRegistration]
        .filter((registration) => registration.surface === request.surface)
        .flatMap((registration) => {
          mappedRegistrations.push(registration)
          const viewer = mapper(registration)
          return viewer ? [viewer] : []
        })
    })

    const rootResource: Resource<Root> = {
      value: null,
      loading: false,
      error: null,
      retry: vi.fn(),
    }

    renderHook(() => useAllViewers(rootResource, 'space/one'))

    expect(mappedRegistrations).toEqual([webRegistration])
  })
})

describe('viewerRegistrationToComponent', () => {
  it('maps dynamic registrations with stable component IDs and display names', () => {
    const viewer = viewerRegistrationToComponent({
      componentId: 'gizmo.worklist.viewer',
      typeId: 'gizmo/worklist',
      viewerName: 'Worklist',
      scriptPath: '/plugins/gizmo/worklist.js',
      category: 'Gizmo',
      surface: ViewerSurface.WEB,
    })

    expect(viewer).toMatchObject({
      componentID: 'gizmo.worklist.viewer',
      typeID: 'gizmo/worklist',
      name: 'Worklist',
      category: 'Gizmo',
    })
  })

  it('rejects dynamic registrations without component IDs', () => {
    expect(
      viewerRegistrationToComponent({
        typeId: 'gizmo/worklist',
        viewerName: 'Worklist',
        scriptPath: '/plugins/gizmo/worklist.js',
        surface: ViewerSurface.WEB,
      }),
    ).toBeNull()
  })
})

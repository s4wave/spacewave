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

describe('useAllViewers', () => {
  it('does not pass terminal registrations to dynamic viewer conversion', () => {
    const webRegistration: ViewerRegistration = {
      componentId: 'glados.workfront.viewer',
      typeId: 'glados/workfront',
      viewerName: 'Workfront',
      scriptPath: '/plugins/glados/workfront.js',
      surface: ViewerSurface.WEB,
    }
    const terminalRegistration: ViewerRegistration = {
      ...webRegistration,
      componentId: 'terminal.workfront.viewer',
      surface: ViewerSurface.TUI,
    }
    const mappedRegistrations: ViewerRegistration[] = []

    h.useDynamicRegistrations.mockImplementationOnce((...args: unknown[]) => {
      const request = args[2] as { surface?: ViewerSurface }
      expect(request).toEqual({ surface: ViewerSurface.WEB })
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

    renderHook(() => useAllViewers(rootResource))

    expect(mappedRegistrations).toEqual([webRegistration])
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
      surface: ViewerSurface.WEB,
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
        surface: ViewerSurface.WEB,
      }),
    ).toBeNull()
  })
})

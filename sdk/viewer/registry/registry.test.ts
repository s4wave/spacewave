import { describe, expect, it } from 'vitest'

import {
  ViewerSurface,
  ViewerRegistration,
  RegisterViewerRequest,
  WatchViewersResponse,
} from './registry.pb.js'

describe('ViewerRegistry proto types', () => {
  it('ViewerRegistration carries a stable component ID separate from display name', () => {
    const registration = ViewerRegistration.create({
      typeId: 'gizmo/worklist',
      viewerName: 'Worklist',
      scriptPath: '/plugins/gizmo/worklist.js',
      category: 'Gizmo',
      componentId: 'gizmo.worklist.viewer',
      surface: ViewerSurface.WEB,
    })

    expect(registration.typeId).toBe('gizmo/worklist')
    expect(registration.viewerName).toBe('Worklist')
    expect(registration.componentId).toBe('gizmo.worklist.viewer')
    expect(registration.surface).toBe(ViewerSurface.WEB)
  })

  it('RegisterViewerRequest round-trips component IDs through binary serialization', () => {
    const original = RegisterViewerRequest.create({
      registration: {
        typeId: 'gizmo/worklist',
        viewerName: 'Worklist',
        scriptPath: '/plugins/gizmo/worklist.js',
        componentId: 'gizmo.worklist.viewer',
        surface: ViewerSurface.WEB,
      },
    })

    const decoded = RegisterViewerRequest.fromBinary(
      RegisterViewerRequest.toBinary(original),
    )

    expect(decoded.registration?.componentId).toBe('gizmo.worklist.viewer')
    expect(decoded.registration?.surface).toBe(ViewerSurface.WEB)
  })

  it('WatchViewersResponse carries registered component IDs', () => {
    const response = WatchViewersResponse.create({
      registrations: [
        {
          typeId: 'gizmo/worklist',
          viewerName: 'Worklist',
          scriptPath: '/plugins/gizmo/worklist.js',
          componentId: 'gizmo.worklist.viewer',
          surface: ViewerSurface.WEB,
        },
      ],
    })

    expect(response.registrations?.[0]?.componentId).toBe(
      'gizmo.worklist.viewer',
    )
    expect(response.registrations?.[0]?.surface).toBe(ViewerSurface.WEB)
  })
})

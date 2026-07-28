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
      typeId: 'glados/workfront',
      viewerName: 'Workfront',
      scriptPath: '/plugins/glados/workfront.js',
      category: 'GLaDOS',
      componentId: 'glados.workfront.viewer',
      surface: ViewerSurface.WEB,
    })

    expect(registration.typeId).toBe('glados/workfront')
    expect(registration.viewerName).toBe('Workfront')
    expect(registration.componentId).toBe('glados.workfront.viewer')
    expect(registration.surface).toBe(ViewerSurface.WEB)
  })

  it('RegisterViewerRequest round-trips component IDs through binary serialization', () => {
    const original = RegisterViewerRequest.create({
      registration: {
        typeId: 'glados/workfront',
        viewerName: 'Workfront',
        scriptPath: '/plugins/glados/workfront.js',
        componentId: 'glados.workfront.viewer',
        surface: ViewerSurface.WEB,
      },
    })

    const decoded = RegisterViewerRequest.fromBinary(
      RegisterViewerRequest.toBinary(original),
    )

    expect(decoded.registration?.componentId).toBe('glados.workfront.viewer')
    expect(decoded.registration?.surface).toBe(ViewerSurface.WEB)
  })

  it('WatchViewersResponse carries registered component IDs', () => {
    const response = WatchViewersResponse.create({
      registrations: [
        {
          typeId: 'glados/workfront',
          viewerName: 'Workfront',
          scriptPath: '/plugins/glados/workfront.js',
          componentId: 'glados.workfront.viewer',
          surface: ViewerSurface.WEB,
        },
      ],
    })

    expect(response.registrations?.[0]?.componentId).toBe(
      'glados.workfront.viewer',
    )
    expect(response.registrations?.[0]?.surface).toBe(ViewerSurface.WEB)
  })
})

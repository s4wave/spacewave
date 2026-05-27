import { describe, expect, it } from 'vitest'

import type { ObjectViewerComponent } from './object.js'
import {
  getDefaultStateNamespace,
  resolveObjectViewerSelection,
} from './useObjectViewer.js'

function component(
  componentID: string,
  name = componentID,
): ObjectViewerComponent {
  return {
    componentID,
    typeID: 'glados/workfront',
    name,
    component: () => null,
  }
}

describe('getDefaultStateNamespace', () => {
  it('uses an explicit namespace over the object-derived default', () => {
    expect(
      getDefaultStateNamespace(
        {
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'drive',
            },
          },
        },
        'drive',
        ['space', 'primary', 'drive'],
      ),
    ).toEqual(['space', 'primary', 'drive'])
  })

  it('keys world object viewer state by object key by default', () => {
    expect(
      getDefaultStateNamespace(
        {
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'drive',
            },
          },
        },
        'drive',
        undefined,
      ),
    ).toEqual(['objectViewer', 'drive'])
  })

  it('keys UnixFS viewer state by UnixFS id and scoped path', () => {
    expect(
      getDefaultStateNamespace(
        {
          info: {
            case: 'unixfsObjectInfo',
            value: {
              unixfsId: 'files',
              path: '/photos/2026',
            },
          },
        },
        undefined,
        undefined,
      ),
    ).toEqual(['objectViewer', 'unixfs', 'files', '/photos/2026'])
  })

  it('changes the UnixFS viewer namespace when the scoped path changes', () => {
    const rootNs = getDefaultStateNamespace(
      {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/',
          },
        },
      },
      undefined,
      undefined,
    )
    const nestedNs = getDefaultStateNamespace(
      {
        info: {
          case: 'unixfsObjectInfo',
          value: {
            unixfsId: 'files',
            path: '/nested',
          },
        },
      },
      undefined,
      undefined,
    )

    expect(rootNs).not.toEqual(nestedNs)
  })
})

describe('resolveObjectViewerSelection', () => {
  it('uses the ObjectLayout preferred component when no local selection exists', () => {
    const viewers = [
      component('spacewave.debug.viewer', 'Debug'),
      component('glados.workfront.viewer', 'Workfront'),
    ]

    expect(
      resolveObjectViewerSelection(
        viewers,
        undefined,
        'glados.workfront.viewer',
      ).selectedComponent?.componentID,
    ).toBe('glados.workfront.viewer')
  })

  it('keeps the browser-local selection ahead of the ObjectLayout preference', () => {
    const viewers = [
      component('spacewave.debug.viewer', 'Debug'),
      component('glados.workfront.viewer', 'Workfront'),
    ]

    expect(
      resolveObjectViewerSelection(
        viewers,
        'spacewave.debug.viewer',
        'glados.workfront.viewer',
      ).selectedComponent?.componentID,
    ).toBe('spacewave.debug.viewer')
  })

  it('does not fall back from component ID selection to display names', () => {
    const viewers = [
      component('spacewave.debug.viewer', 'glados.workfront.viewer'),
      component('glados.workfront.viewer', 'Workfront'),
    ]

    expect(
      resolveObjectViewerSelection(
        viewers,
        undefined,
        'glados.workfront.viewer',
      ).selectedComponent?.name,
    ).toBe('Workfront')
  })

  it('falls back to the first visible viewer and exposes missing component IDs', () => {
    const viewers = [
      component('spacewave.debug.viewer', 'Debug'),
      component('glados.workfront.viewer', 'Workfront'),
    ]

    const selection = resolveObjectViewerSelection(
      viewers,
      undefined,
      'missing.viewer',
    )

    expect(selection.selectedComponent?.componentID).toBe(
      'spacewave.debug.viewer',
    )
    expect(selection.missingComponentID).toBe('missing.viewer')
  })

  it('uses the authored component when a plugin reload makes it visible', () => {
    const beforeReload = [component('spacewave.debug.viewer', 'Debug')]
    const afterReload = [
      component('spacewave.debug.viewer', 'Debug'),
      component('glados.decision', 'Decision'),
    ]

    expect(
      resolveObjectViewerSelection(beforeReload, undefined, 'glados.decision'),
    ).toMatchObject({
      selectedComponent: { componentID: 'spacewave.debug.viewer' },
      missingComponentID: 'glados.decision',
    })
    expect(
      resolveObjectViewerSelection(afterReload, undefined, 'glados.decision'),
    ).toMatchObject({
      selectedComponent: { componentID: 'glados.decision' },
      missingComponentID: undefined,
    })
  })
})

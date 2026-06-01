import { cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  ObjectTypeVisibility,
  type ObjectTypeMetadata,
} from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import { buildObjectTypeMetadataMap } from '@s4wave/web/space/object-tree.js'

import type { ObjectViewerComponent } from './object.js'
import {
  getDefaultStateNamespace,
  resolveObjectViewerSelection,
  shouldHoldDebugViewerFallback,
  useObjectViewer,
} from './useObjectViewer.js'

const h = vi.hoisted(() => {
  const metadata: ReadonlyMap<string, ObjectTypeMetadata> = new Map()
  return {
    metadata,
    worldSetup: {
      typeID: 'canvas',
      rootRef: '',
      visibleComponents: [],
      objectState: {
        value: { id: 'visible-doc' },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
    },
    spaceContext: {
      spaceState: {
        ready: true,
        worldContents: {
          objects: [
            { objectKey: 'visible-doc', objectType: 'canvas' },
            { objectKey: 'internal', objectType: 'plugin/internal' },
            { objectKey: 'hidden', objectType: 'plugin/hidden' },
          ],
        },
      },
      navigateToObjects: vi.fn(),
    },
  }
})

vi.mock('@s4wave/web/hooks/useObjectTypeMetadata.js', () => ({
  useObjectTypeMetadata: () => h.metadata,
}))

vi.mock('@s4wave/web/hooks/useViewerRegistry.js', () => ({
  useAllViewers: () => [],
  getViewersForType: () => [],
}))

vi.mock('./useObjectViewerSetup.js', () => ({
  useObjectViewerSetup: () => h.worldSetup,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => h.spaceContext,
  },
}))

beforeEach(() => {
  h.metadata = new Map()
  h.worldSetup = {
    typeID: 'canvas',
    rootRef: '',
    visibleComponents: [],
    objectState: {
      value: { id: 'visible-doc' },
      loading: false,
      error: null,
      retry: vi.fn(),
    },
  }
  h.spaceContext = {
    spaceState: {
      ready: true,
      worldContents: {
        objects: [
          { objectKey: 'visible-doc', objectType: 'canvas' },
          { objectKey: 'internal', objectType: 'plugin/internal' },
          { objectKey: 'hidden', objectType: 'plugin/hidden' },
        ],
      },
    },
    navigateToObjects: vi.fn(),
  }
})

afterEach(() => {
  cleanup()
})

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

describe('shouldHoldDebugViewerFallback', () => {
  it('holds the debug viewer when it was only selected as a wildcard fallback', () => {
    expect(
      shouldHoldDebugViewerFallback(
        'glados/unknown',
        component('spacewave.debug.viewer', 'Debug'),
        undefined,
        undefined,
      ),
    ).toBe(true)
  })

  it('allows the debug viewer when the user selected it directly', () => {
    expect(
      shouldHoldDebugViewerFallback(
        'glados/unknown',
        component('spacewave.debug.viewer', 'Debug'),
        'spacewave.debug.viewer',
        undefined,
      ),
    ).toBe(false)
  })

  it('allows the debug viewer when a layout requested it directly', () => {
    expect(
      shouldHoldDebugViewerFallback(
        'glados/unknown',
        component('spacewave.debug.viewer', 'Debug'),
        undefined,
        'spacewave.debug.viewer',
      ),
    ).toBe(false)
  })

  it('does not hold a real viewer selection', () => {
    expect(
      shouldHoldDebugViewerFallback(
        'glados/workfront',
        component('glados.workfront.viewer', 'Workfront'),
        undefined,
        undefined,
      ),
    ).toBe(false)
  })
})

describe('useObjectViewer context menu targets', () => {
  it('filters hidden and internal object metadata from object navigation actions', () => {
    h.metadata = buildObjectTypeMetadataMap([
      {
        typeId: 'plugin/internal',
        registrationId: 1,
        metadata: { visibility: ObjectTypeVisibility.INTERNAL },
      },
      {
        typeId: 'plugin/hidden',
        registrationId: 2,
        metadata: { visibility: ObjectTypeVisibility.HIDDEN },
      },
    ])

    const { result } = renderHook(() =>
      useObjectViewer({
        objectInfo: {
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'visible-doc',
              objectType: 'canvas',
            },
          },
        },
        worldState: emptyWorldState,
      }),
    )

    const labels = JSON.stringify(result.current.contextMenuItems)
    expect(labels).toContain('Visible Doc')
    expect(labels).not.toContain('Internal')
    expect(labels).not.toContain('hidden')
  })
})

const emptyWorldState: Resource<IWorldState> = {
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
}

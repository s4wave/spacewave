import { cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import type { ObjectViewerComponent } from './object.js'
import { useObjectViewerSetup } from './useObjectViewerSetup.js'

interface LoadedObjectInfo {
  key: string
  rootRef: string
  typeID: string
}

const h = vi.hoisted(() => ({
  allViewers: [] as ObjectViewerComponent[],
  objectInfo: {
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  } as Resource<LoadedObjectInfo | null>,
  objectState: {
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  } as Resource<IObjectState | null>,
  rootResource: {
    value: {},
    loading: false,
    error: null,
    retry: vi.fn(),
  } as Resource<{}>,
  useResource: vi.fn(),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: h.useResource,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  RootContext: {
    useContext: () => h.rootResource,
  },
}))

vi.mock('@s4wave/web/hooks/useViewerRegistry.js', () => ({
  useAllViewers: () => h.allViewers,
  getViewersForType: (typeID: string, viewers: typeof h.allViewers) =>
    viewers.filter((viewer) => viewer.typeID === typeID),
}))

vi.mock('@s4wave/sdk/world/object-ref.js', () => ({
  formatObjectRef: vi.fn(),
}))

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  getObjectType: vi.fn(),
}))

beforeEach(() => {
  h.allViewers = []
  h.objectInfo = resource<LoadedObjectInfo | null>(null)
  h.objectState = resource<IObjectState | null>(null)
  h.rootResource = resource({})
  h.useResource.mockReset()
  h.useResource.mockImplementation((source: unknown) =>
    Array.isArray(source) ? h.objectInfo : h.objectState,
  )
})

afterEach(() => {
  cleanup()
})

function resource<T>(value: T, loading = false): Resource<T> {
  return {
    value,
    loading,
    error: null,
    retry: vi.fn(),
  }
}

function viewer(componentID: string, typeID: string): ObjectViewerComponent {
  return {
    componentID,
    typeID,
    name: componentID,
    component: (() => null) as ObjectViewerComponent['component'],
  }
}

describe('useObjectViewerSetup route transitions', () => {
  it('does not expose the previous object as ready while the next object is loading', () => {
    const previousObject = {
      getKey: () => 'world/object-a',
    } as IObjectState
    const nextObject = {
      getKey: () => 'world/object-b',
    } as IObjectState
    const worldState = resource({} as IWorldState)

    h.allViewers = [
      viewer('viewer.previous', 'type/previous'),
      viewer('viewer.route-hint', 'type/route-hint'),
      viewer('viewer.current', 'type/current'),
    ]
    h.objectState = resource<IObjectState | null>(previousObject)
    h.objectInfo = resource<LoadedObjectInfo | null>({
      key: 'world/object-a',
      typeID: 'type/previous',
      rootRef: 'root://previous',
    })

    const initialProps: {
      objectKey: string
      typeIDHint: string | undefined
    } = {
      objectKey: 'world/object-a',
      typeIDHint: undefined,
    }
    const { result, rerender } = renderHook(
      ({ objectKey, typeIDHint }: { objectKey: string; typeIDHint?: string }) =>
        useObjectViewerSetup(worldState, objectKey, { typeIDHint }),
      { initialProps },
    )

    expect(result.current.objectState.value).toBe(previousObject)
    expect(result.current.objectState.loading).toBe(false)
    expect(result.current.typeID).toBe('type/previous')
    expect(result.current.rootRef).toBe('root://previous')
    expect(
      result.current.visibleComponents.map(
        (component) => component.componentID,
      ),
    ).toEqual(['viewer.previous'])

    h.objectState = resource<IObjectState | null>(previousObject, true)
    h.objectInfo = resource<LoadedObjectInfo | null>(
      {
        key: 'world/object-a',
        typeID: 'type/previous',
        rootRef: 'root://previous',
      },
      true,
    )

    rerender({
      objectKey: 'world/object-b',
      typeIDHint: 'type/route-hint',
    })

    expect(result.current.objectState.value).toBeNull()
    expect(result.current.objectState.loading).toBe(true)
    expect(result.current.typeID).toBe('type/route-hint')
    expect(result.current.rootRef).toBeUndefined()
    expect(
      result.current.visibleComponents.map(
        (component) => component.componentID,
      ),
    ).toEqual(['viewer.route-hint'])

    h.objectState = resource<IObjectState | null>(nextObject)
    h.objectInfo = resource<LoadedObjectInfo | null>({
      key: 'world/object-b',
      typeID: 'type/current',
      rootRef: 'root://current',
    })

    rerender({
      objectKey: 'world/object-b',
      typeIDHint: 'type/route-hint',
    })

    expect(result.current.objectState.value).toBe(nextObject)
    expect(result.current.objectState.loading).toBe(false)
    expect(result.current.typeID).toBe('type/current')
    expect(result.current.rootRef).toBe('root://current')
    expect(
      result.current.visibleComponents.map(
        (component) => component.componentID,
      ),
    ).toEqual(['viewer.current'])
  })
})

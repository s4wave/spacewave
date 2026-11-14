import { lazy, type ComponentType } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'
import {
  SpacePluginLifecycleState,
  type SpaceContentsState,
} from '@s4wave/sdk/space/space.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { ObjectViewerContent } from './ObjectViewerContent.js'
import type {
  ObjectViewerComponent,
  ObjectViewerComponentProps,
} from './object.js'
import type { ObjectInfo } from './object.pb.js'

function buildWorldState(): Resource<IWorldState> {
  return {
    value: {} as IWorldState,
    loading: false,
    error: null,
    retry: () => {},
  }
}

function buildObjectInfo(): ObjectInfo {
  return {
    info: {
      case: 'worldObjectInfo',
      value: {
        objectKey: 'gizmo/bootstrap/llm-session',
        objectType: 'gizmo/llm-session',
      },
    },
  }
}

// buildSpaceContents returns Space contents whose state stream yields each
// pushed state.
function buildSpaceContents(): {
  resource: Resource<SpaceContents>
  push: (state: SpaceContentsState) => void
} {
  const states: SpaceContentsState[] = []
  let wake: (() => void) | null = null
  const contents = {
    async *watchState(_req: unknown, signal: AbortSignal) {
      while (!signal.aborted) {
        const state = states.shift()
        if (state) {
          yield state
          continue
        }
        await new Promise<void>((resolve) => {
          wake = resolve
        })
      }
    },
  } as unknown as SpaceContents
  return {
    resource: { value: contents, loading: false, error: null, retry: () => {} },
    push: (state) => {
      states.push(state)
      wake?.()
    },
  }
}

describe('ObjectViewerContent', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('contains lazy viewer suspension inside the object viewer pane', () => {
    const PendingViewer = lazy(
      () =>
        new Promise<{
          default: ComponentType<ObjectViewerComponentProps>
        }>(() => {}),
    )
    const component: ObjectViewerComponent = {
      componentID: 'gizmo.llm-session.viewer',
      typeID: 'gizmo/llm-session',
      name: 'LlmSession',
      component: PendingViewer,
    }

    render(
      <ObjectViewerContent
        objectInfo={buildObjectInfo()}
        worldState={buildWorldState()}
        typeID="gizmo/llm-session"
        component={component}
      />,
    )

    expect(screen.getByText('Loading the object')).toBeDefined()
    expect(screen.queryByText('LlmSession loaded')).toBeNull()
  })

  it('contains lazy viewer import errors inside the object viewer pane', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const BrokenViewer = lazy(() =>
      Promise.reject<{
        default: ComponentType<ObjectViewerComponentProps>
      }>(
        new Error(
          'Failed to fetch dynamically imported module: app://index.html/b/pa/gizmo-web/v/b/fe/web/worklist/WorklistViewer-DemfR9tb.mjs',
        ),
      ),
    )
    const component: ObjectViewerComponent = {
      componentID: 'gizmo.worklist.viewer',
      typeID: 'gizmo/worklist',
      name: 'Worklist',
      component: BrokenViewer,
    }

    render(
      <ObjectViewerContent
        objectInfo={buildObjectInfo()}
        worldState={buildWorldState()}
        typeID="gizmo/worklist"
        component={component}
      />,
    )

    expect(await screen.findByText('Failed to load module')).toBeDefined()
    expect(
      screen.getByText(
        '/b/pa/gizmo-web/v/b/fe/web/worklist/WorklistViewer-DemfR9tb.mjs',
      ),
    ).toBeDefined()
  })

  it('shows a recovery state before opening the debug viewer fallback', () => {
    const onSelectComponent = vi.fn()
    const debugComponent: ObjectViewerComponent = {
      componentID: 'spacewave.debug.viewer',
      typeID: '*',
      name: 'Debug Viewer',
      component: () => null,
    }

    render(
      <ObjectViewerContent
        objectInfo={buildObjectInfo()}
        worldState={buildWorldState()}
        typeID="gizmo/missing"
        availableComponents={[debugComponent]}
        missingComponentID="gizmo.custom.viewer"
        onSelectComponent={onSelectComponent}
      />,
    )

    expect(screen.getByText("Can't open this object yet")).toBeDefined()
    expect(screen.getByText('About this object')).toBeDefined()
    expect(screen.getByText('Object key')).toBeDefined()
    expect(screen.getByTitle('gizmo/bootstrap/llm-session')).toBeDefined()
    expect(screen.getByText('Object type')).toBeDefined()
    expect(screen.getByTitle('gizmo/missing')).toBeDefined()
    expect(screen.getByText(/gizmo\.custom\.viewer/)).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Open raw object' }))

    expect(onSelectComponent).toHaveBeenCalledWith(debugComponent)
  })

  it('waits for a loading Space plugin before reporting a missing viewer', async () => {
    const contents = buildSpaceContents()
    render(
      <ObjectViewerContent
        objectInfo={buildObjectInfo()}
        worldState={buildWorldState()}
        spaceContents={contents.resource}
        typeID="gizmo/missing"
      />,
    )

    contents.push({
      ready: true,
      plugins: [
        {
          pluginId: 'gizmo',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING,
        },
      ],
    })
    expect(
      await screen.findByText(
        'Loading the Space plugins that open this object',
      ),
    ).toBeDefined()

    contents.push({
      ready: true,
      plugins: [
        {
          pluginId: 'gizmo',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
        },
      ],
    })
    expect(await screen.findByText("Can't open this object yet")).toBeDefined()
  })
})

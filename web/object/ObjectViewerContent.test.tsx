import { lazy, type ComponentType } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
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
        objectKey: 'glados/bootstrap/llm-session',
        objectType: 'glados/llm-session',
      },
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
      componentID: 'glados.llm-session.viewer',
      typeID: 'glados/llm-session',
      name: 'LlmSession',
      component: PendingViewer,
    }

    render(
      <ObjectViewerContent
        objectInfo={buildObjectInfo()}
        worldState={buildWorldState()}
        typeID="glados/llm-session"
        component={component}
      />,
    )

    expect(screen.getByText('Loading object')).toBeDefined()
    expect(screen.queryByText('LlmSession loaded')).toBeNull()
  })

  it('contains lazy viewer import errors inside the object viewer pane', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const BrokenViewer = lazy(() =>
      Promise.reject<{
        default: ComponentType<ObjectViewerComponentProps>
      }>(
        new Error(
          'Failed to fetch dynamically imported module: app://index.html/b/pa/glados-web/v/b/fe/web/workfront/WorkfrontViewer-DemfR9tb.mjs',
        ),
      ),
    )
    const component: ObjectViewerComponent = {
      componentID: 'glados.workfront.viewer',
      typeID: 'glados/workfront',
      name: 'Workfront',
      component: BrokenViewer,
    }

    render(
      <ObjectViewerContent
        objectInfo={buildObjectInfo()}
        worldState={buildWorldState()}
        typeID="glados/workfront"
        component={component}
      />,
    )

    expect(await screen.findByText('Failed to load module')).toBeDefined()
    expect(
      screen.getByText(
        '/b/pa/glados-web/v/b/fe/web/workfront/WorkfrontViewer-DemfR9tb.mjs',
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
        typeID="glados/missing"
        availableComponents={[debugComponent]}
        missingComponentID="glados.custom.viewer"
        onSelectComponent={onSelectComponent}
      />,
    )

    expect(screen.getByText("Can't open this object yet")).toBeDefined()
    expect(screen.getByText('About this object')).toBeDefined()
    expect(screen.getByText('Object key')).toBeDefined()
    expect(screen.getByText('glados/bootstrap/llm-session')).toBeDefined()
    expect(screen.getByText('Object type')).toBeDefined()
    expect(screen.getByText('glados/missing')).toBeDefined()
    expect(screen.getByText(/glados\.custom\.viewer/)).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Open raw object' }))

    expect(onSelectComponent).toHaveBeenCalledWith(debugComponent)
  })
})

import { lazy, type ComponentType } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

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
  })

  it('contains lazy viewer suspension inside the object viewer pane', () => {
    const PendingViewer = lazy(
      () =>
        new Promise<{
          default: ComponentType<ObjectViewerComponentProps>
        }>(() => {}),
    )
    const component: ObjectViewerComponent = {
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
})

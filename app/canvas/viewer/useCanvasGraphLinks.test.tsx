import { Window } from 'happy-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, renderHook, waitFor } from '@testing-library/react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

import type { CanvasStateData } from '../types.js'
import { useCanvasGraphLinks } from './useCanvasGraphLinks.js'

Object.defineProperties(globalThis, {
  document: { value: new Window().document, configurable: true },
  window: { value: new Window(), configurable: true },
})

afterEach(cleanup)

describe('useCanvasGraphLinks', () => {
  it('contains graph query failures in recoverable state', async () => {
    const canvasState: CanvasStateData = {
      nodes: new Map([
        [
          'node-1',
          {
            id: 'node-1',
            type: 'world_object',
            objectKey: 'docs',
            x: 0,
            y: 0,
            width: 100,
            height: 100,
            zIndex: 0,
          },
        ],
      ]),
      edges: [],
      hiddenGraphLinks: [],
      layoutMetadata: new Map(),
    }
    const worldState = {
      value: {
        listGraphEdgeBuckets: vi.fn().mockRejectedValue(new Error('offline')),
      },
      loading: false,
      error: null,
      retry: vi.fn(),
    } as unknown as ObjectViewerComponentProps['worldState']
    const { result } = renderHook(() =>
      useCanvasGraphLinks({
        canvasState,
        graphLinkObjectMetadata: new Map(),
        hiddenGraphLinks: [],
        nodesByObjectKey: new Map(),
        refreshToken: 0,
        selectedNodeIds: new Set(['node-1']),
        worldState,
      }),
    )

    await waitFor(() =>
      expect(result.current.error).toContain('could not load'),
    )
    expect(result.current.edges).toEqual([])
  })
})

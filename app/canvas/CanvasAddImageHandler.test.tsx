import React from 'react'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const { capturedCommands } = vi.hoisted(() => ({
  capturedCommands: {
    current: null as null | { onAddImage?: (path: string) => void },
  },
}))

vi.mock('./useCanvasCommands.js', () => ({
  useCanvasCommands: (params: { onAddImage?: (path: string) => void }) => {
    capturedCommands.current = params
  },
}))

vi.mock('./CanvasContextMenu.js', () => ({
  CanvasContextMenu: () => null,
}))

import { Canvas } from './Canvas.js'
import type { CanvasCallbacks, CanvasStateData } from './types.js'

const emptyState: CanvasStateData = {
  nodes: new Map(),
  edges: [],
  hiddenGraphLinks: [],
  layoutMetadata: new Map(),
}

describe('Canvas add-image handler', () => {
  afterEach(() => {
    cleanup()
    capturedCommands.current = null
  })

  it('places the selected UnixFS image as a world-object viewer node', () => {
    const onNodesChange = vi.fn<NonNullable<CanvasCallbacks['onNodesChange']>>()
    render(
      <Canvas
        state={emptyState}
        callbacks={{ onNodesChange }}
        imageObjectKey="drive"
        imageSubItems={() => Promise.resolve([])}
      />,
    )

    act(() => capturedCommands.current?.onAddImage?.('/photos/cat.png'))

    const nodes = onNodesChange.mock.calls[0]?.[0]
    expect(nodes?.size).toBe(1)
    const node = nodes ? [...nodes.values()][0] : undefined
    expect(node).toMatchObject({
      type: 'world_object',
      objectKey: 'drive',
      viewPath: '/photos/cat.png',
      pinned: true,
      width: 400,
      height: 300,
    })
  })
})

import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { Canvas } from './Canvas.js'
import type {
  CanvasCallbacks,
  CanvasNodeData,
  CanvasStateData,
} from './types.js'

function makeState(nodes: CanvasNodeData[] = []): CanvasStateData {
  const nodeMap = new Map<string, CanvasNodeData>()
  for (const n of nodes) {
    nodeMap.set(n.id, n)
  }
  return { nodes: nodeMap, edges: [], hiddenGraphLinks: [] }
}

function makeCallbacks(
  overrides: Partial<CanvasCallbacks> = {},
): CanvasCallbacks {
  return {
    onNodesChange: vi.fn(),
    onEdgesChange: vi.fn(),
    onNodeSelect: vi.fn(),
    ...overrides,
  }
}

describe('Canvas real context-menu interactions', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('adds a text node from the real background context menu', async () => {
    const user = userEvent.setup()
    const setTimeoutSpy = vi.spyOn(window, 'setTimeout')

    render(<Canvas state={makeState()} callbacks={makeCallbacks()} />)

    fireEvent.contextMenu(screen.getByTestId('canvas-viewport'), {
      clientX: 180,
      clientY: 220,
    })

    await user.click(screen.getByText('Add Text Node Here'))
    expect(setTimeoutSpy).toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.getByRole('textbox')).toBeTruthy()
    })
  })
})

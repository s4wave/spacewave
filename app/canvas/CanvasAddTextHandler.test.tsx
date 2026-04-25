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

const { onAddTextClick } = vi.hoisted(() => ({
  onAddTextClick: vi.fn(),
}))

vi.mock('./CanvasContextMenu.js', () => ({
  CanvasContextMenu: ({
    state,
    onAddText,
  }: {
    state: { position: { x: number; y: number } } | null
    onAddText: () => void
  }) =>
    state ?
      <button
        type="button"
        onClick={() => {
          onAddTextClick()
          onAddText()
        }}
      >
        Add Text Node Here
      </button>
    : null,
}))

vi.mock('./CanvasTextNode.js', () => ({
  CanvasTextNode: () => <div data-testid="pending-text-editor" />,
}))

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

describe('Canvas add-text handler', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders pending text when the menu callback fires', async () => {
    const user = userEvent.setup()

    render(<Canvas state={makeState()} callbacks={makeCallbacks()} />)

    fireEvent.contextMenu(screen.getByTestId('canvas-viewport'), {
      clientX: 180,
      clientY: 220,
    })

    const addTextButton = screen.getByRole('button', {
      name: 'Add Text Node Here',
    })
    await user.click(addTextButton)
    expect(onAddTextClick).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(screen.getByTestId('pending-text-editor')).toBeTruthy()
    })
  })
})

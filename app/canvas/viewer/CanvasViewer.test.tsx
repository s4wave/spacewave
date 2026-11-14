import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { isCanvasInsertableObject } from './object-picker.js'

interface WatchResource {
  value: { nodes: Record<string, { x: number; y: number }> } | null
  loading: boolean
  error: Error | null
  retry: () => void
}

const h = vi.hoisted(() => ({
  watch: null as WatchResource | null,
  canvasNodes: vi.fn((_count: number) => {}),
}))

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: null,
    loading: false,
    error: null,
    retry: () => {},
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => h.watch,
}))

vi.mock('@s4wave/web/hooks/useWorldQuery.js', () => ({
  useWorldQuery: () => ({ value: null, loading: false, error: null }),
}))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: () => ({ value: null, loading: false, error: null }),
}))

vi.mock('../Canvas.js', () => ({
  Canvas: ({ state }: { state: { nodes: Map<string, unknown> } }) => {
    h.canvasNodes(state.nodes.size)
    return <div data-testid="canvas" />
  },
}))

import { CanvasViewer } from './CanvasViewer.js'

function renderViewer() {
  return render(
    <CanvasViewer
      objectInfo={
        {
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'canvas/main' },
          },
        } as never
      }
      worldState={{ value: null, loading: false, error: null } as never}
    />,
  )
}

describe('CanvasViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.watch = null
  })

  it('keeps the canvas on the stage under one notice when a read fails', () => {
    const retry = vi.fn()
    h.watch = {
      value: { nodes: { a: { x: 1, y: 2 } } },
      loading: false,
      error: new Error('stream closed'),
      retry,
    }
    renderViewer()

    expect(screen.getByTestId('canvas')).toBeTruthy()
    expect(h.canvasNodes).toHaveBeenLastCalledWith(1)
    expect(
      screen.getAllByText('Could not read the canvas: stream closed'),
    ).toHaveLength(1)
    fireEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(retry).toHaveBeenCalledTimes(1)
  })

  it('offers Retry in place of the stage when the first read fails', () => {
    const retry = vi.fn()
    h.watch = {
      value: null,
      loading: false,
      error: new Error('no handle'),
      retry,
    }
    renderViewer()

    expect(screen.queryByTestId('canvas')).toBeNull()
    expect(
      screen.getByText('Could not read the canvas: no handle'),
    ).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(retry).toHaveBeenCalledTimes(1)
  })
})

describe('isCanvasInsertableObject', () => {
  it('hides the reserved space settings object', () => {
    expect(
      isCanvasInsertableObject('settings', 'space/settings', 'canvas-1'),
    ).toBe(false)
  })

  it('keeps regular objects available for insertion', () => {
    expect(
      isCanvasInsertableObject(
        'object-layout/main',
        'alpha/object-layout',
        'canvas-1',
      ),
    ).toBe(true)
  })
})

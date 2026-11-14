import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { readOnlyMessage } from './CanvasView.js'
import { isCanvasInsertableObject } from './object-picker.js'

// CanvasProto is the slice of the canvas watch's message the viewer reads.
interface CanvasProto {
  nodes: Record<string, { x: number; y: number }>
}

// Handle is the slice of the canvas handle the viewer calls.
interface Handle {
  update: () => Promise<void>
  watchState: (signal: AbortSignal) => AsyncIterable<CanvasProto>
}

// HandleResource is the handle as useAccessTypedHandle reports it.
interface HandleResource {
  value: Handle | null
  loading: boolean
  error: Error | null
  retry: () => void
}

interface CanvasProps {
  state: { nodes: Map<string, { x: number; y: number }> }
  callbacks: { onNodesChange?: (nodes: Map<string, unknown>) => void }
  readOnly?: boolean
}

const h = vi.hoisted(() => ({
  handleResource: null as HandleResource | null,
  canvas: null as CanvasProps | null,
  canvasNodes: vi.fn((_count: number) => {}),
}))

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => h.handleResource,
}))

vi.mock('@s4wave/web/hooks/useWorldQuery.js', () => ({
  useWorldQuery: () => ({ value: null, loading: false, error: null }),
}))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: () => ({ value: null, loading: false, error: null }),
}))

vi.mock('../Canvas.js', () => ({
  Canvas: (props: CanvasProps) => {
    h.canvas = props
    h.canvasNodes(props.state.nodes.size)
    return <div data-testid="canvas" />
  },
}))

import { CanvasViewer } from './CanvasViewer.js'

// openHandle is a handle whose watch yields one canvas with node a and then
// stays open, and whose save never answers, so an edit stays pending.
function openHandle(): Handle {
  return {
    update: vi.fn(() => new Promise<void>(() => {})),
    async *watchState(signal) {
      yield { nodes: { a: { x: 1, y: 2 } } }
      await new Promise((resolve) =>
        signal.addEventListener('abort', resolve, { once: true }),
      )
    },
  }
}

// handleState is the handle resource in one state.
function handleState(
  value: Handle | null,
  error: Error | null = null,
  retry: () => void = () => {},
): HandleResource {
  return { value, loading: false, error, retry }
}

function viewer() {
  return (
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
    />
  )
}

// renderWithCanvas renders the viewer on an open handle and waits for the
// watch's first canvas, through the real useStreamingResource.
async function renderWithCanvas(handle: Handle) {
  h.handleResource = handleState(handle)
  const view = render(viewer())
  await screen.findByTestId('canvas')
  return view
}

describe('CanvasViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.handleResource = null
    h.canvas = null
  })

  it('keeps the canvas and queued edits through a handle failure and Retry', async () => {
    const handle = openHandle()
    const { rerender } = await renderWithCanvas(handle)

    // The edit goes to the real queue while the handle is open; its save
    // never answers, so it stays pending.
    const moved = { ...h.canvas!.state.nodes.get('a')!, x: 300, y: 400 }
    const added = { ...moved, id: 'b', x: 50, y: 60 }
    act(() => {
      h.canvas!.callbacks.onNodesChange!(
        new Map<string, unknown>([
          ['a', moved],
          ['b', added],
        ]),
      )
    })
    expect(handle.update).toHaveBeenCalledTimes(1)
    expect(h.canvas!.readOnly).toBe(false)

    // The handle fails: the watch keeps its canvas as last, drawn with the
    // queued edits under the handle's failure and its Retry. With no handle
    // to send on, the canvas is read-only and says why.
    const retry = vi.fn()
    h.handleResource = handleState(null, new Error('stream reset'), retry)
    rerender(viewer())
    expect(screen.getByTestId('canvas')).toBeTruthy()
    expect(h.canvas!.state.nodes.size).toBe(2)
    expect(
      screen.getAllByText('Could not read the canvas: stream reset'),
    ).toHaveLength(1)
    expect(h.canvas!.readOnly).toBe(true)
    expect(screen.getByText(readOnlyMessage)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(retry).toHaveBeenCalledTimes(1)

    // A new handle yields the canvas; the queued edits draw on it, unsent,
    // and editing returns.
    const reopened = openHandle()
    h.handleResource = handleState(reopened)
    rerender(viewer())
    await screen.findByTestId('canvas')
    const drawn = h.canvas!.state.nodes
    expect(drawn.size).toBe(2)
    expect(drawn.get('a')).toMatchObject({ x: 300, y: 400 })
    expect(drawn.get('b')).toMatchObject({ x: 50, y: 60 })
    expect(reopened.update).not.toHaveBeenCalled()
    expect(h.canvas!.readOnly).toBe(false)
    expect(screen.queryByText(readOnlyMessage)).toBeNull()
  })

  it('draws a failed save over the stage, leaving the stage unchanged', async () => {
    const handle = openHandle()
    handle.update = vi.fn(() => Promise.reject(new Error('denied')))
    const { container } = await renderWithCanvas(handle)
    const stage = container.firstElementChild!
    const stageClass = stage.className

    // The save fails through the real queue, which rolls the edit back.
    const moved = { ...h.canvas!.state.nodes.get('a')!, x: 300, y: 400 }
    act(() => {
      h.canvas!.callbacks.onNodesChange!(new Map([['a', moved]]))
    })
    await screen.findByText(/Could not save the last change/)

    // The same stage element stays the pane's only child with the same box
    // classes, and the notice sits inside it, absolutely placed.
    expect(container.firstElementChild).toBe(stage)
    expect(container.childElementCount).toBe(1)
    expect(stage.className).toBe(stageClass)
    const notices = screen.getByTestId('canvas-notices')
    expect(stage.contains(notices)).toBe(true)
    expect(notices.className).toMatch(/\babsolute\b/)

    // A later edit leaves the notice up; Dismiss clears it.
    act(() => {
      h.canvas!.callbacks.onNodesChange!(new Map([['a', moved]]))
    })
    expect(screen.getByText(/Could not save the last change/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryByTestId('canvas-notices')).toBeNull()
  })

  it('keeps the canvas under the failure once the watch reaches its backoff cap', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      // The first watch yields a canvas and drops; every reopen fails.
      let opened = 0
      const handle: Handle = {
        update: vi.fn(() => Promise.resolve()),
        async *watchState() {
          opened++
          if (opened === 1) {
            yield { nodes: { a: { x: 1, y: 2 } } }
            await Promise.resolve()
          }
          throw new Error('stream reset')
        },
      }
      h.handleResource = handleState(handle)
      render(viewer())
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      expect(screen.getByTestId('canvas')).toBeTruthy()

      // Below the cap a line says the canvas reads again; at the cap the
      // failure shows with Retry over the same canvas, never the empty hint.
      for (let i = 0; i < 7; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(5000)
        })
      }
      expect(opened).toBeGreaterThanOrEqual(7)
      expect(screen.getByTestId('canvas')).toBeTruthy()
      expect(h.canvas!.state.nodes.get('a')).toMatchObject({ x: 1, y: 2 })
      expect(
        screen.getByText('Could not read the canvas: stream reset'),
      ).toBeTruthy()
      expect(screen.queryByText(/Empty canvas/)).toBeNull()

      // Retry reopens at once and keeps the canvas drawn.
      const before = opened
      fireEvent.click(screen.getByRole('button', { name: /retry/i }))
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      expect(opened).toBe(before + 1)
      expect(screen.getByTestId('canvas')).toBeTruthy()
    } finally {
      vi.useRealTimers()
    }
  })

  it('offers Retry in place of the stage when the first read fails', () => {
    const retry = vi.fn()
    h.handleResource = handleState(null, new Error('no handle'), retry)
    render(viewer())

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

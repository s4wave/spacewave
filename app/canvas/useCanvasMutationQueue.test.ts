import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, cleanup, act } from '@testing-library/react'

import { useCanvasMutationQueue } from './useCanvasMutationQueue.js'
import type {
  CanvasStateData,
  CanvasNodeData,
  CanvasEdgeData,
  HiddenGraphLinkData,
} from './types.js'

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'n1',
    x: 0,
    y: 0,
    width: 200,
    height: 150,
    zIndex: 0,
    type: 'text',
    ...overrides,
  }
}

function makeState(
  nodes: [string, CanvasNodeData][],
  edges: CanvasEdgeData[] = [],
  hiddenGraphLinks: HiddenGraphLinkData[] = [],
): CanvasStateData {
  return { nodes: new Map(nodes), edges, hiddenGraphLinks }
}

describe('useCanvasMutationQueue', () => {
  afterEach(() => {
    cleanup()
  })

  it('returns server state when queue is empty', () => {
    const state = makeState([['a', makeNode({ id: 'a', x: 10 })]])
    const { result } = renderHook(() => useCanvasMutationQueue(state, null))

    expect(result.current.effectiveState).toBe(state)
    expect(result.current.pending).toBe(0)
  })

  it('returns empty state when server state is null', () => {
    const { result } = renderHook(() => useCanvasMutationQueue(null, null))

    expect(result.current.effectiveState.nodes.size).toBe(0)
    expect(result.current.effectiveState.edges.length).toBe(0)
    expect(result.current.effectiveState.hiddenGraphLinks.length).toBe(0)
  })

  it('applies node change mutation on top of server state', () => {
    const state = makeState([['a', makeNode({ id: 'a', x: 10 })]])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      result.current.enqueueNodesChange(
        new Map([['a', makeNode({ id: 'a', x: 50 })]]),
      )
    })

    expect(result.current.pending).toBe(1)
    expect(result.current.effectiveState.nodes.get('a')?.x).toBe(50)
    expect(send).toHaveBeenCalledTimes(1)
  })

  it('applies node removal mutation', () => {
    const state = makeState([
      ['a', makeNode({ id: 'a' })],
      ['b', makeNode({ id: 'b' })],
    ])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      result.current.enqueueNodesRemove(['a'])
    })

    expect(result.current.effectiveState.nodes.has('a')).toBe(false)
    expect(result.current.effectiveState.nodes.has('b')).toBe(true)
  })

  it('applies edge addition mutation', () => {
    const state = makeState([['a', makeNode({ id: 'a' })]])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    const edge: CanvasEdgeData = {
      id: 'e1',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: 'bezier',
    }

    act(() => {
      result.current.enqueueEdgesAdd([edge])
    })

    expect(result.current.effectiveState.edges).toHaveLength(1)
    expect(result.current.effectiveState.edges[0].id).toBe('e1')
  })

  it('deduplicates added edges by ID', () => {
    const edge: CanvasEdgeData = {
      id: 'e1',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: 'bezier',
    }
    const state: CanvasStateData = {
      nodes: new Map(),
      edges: [edge],
      hiddenGraphLinks: [],
    }
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      result.current.enqueueEdgesAdd([edge])
    })

    // Should not duplicate the edge.
    expect(result.current.effectiveState.edges).toHaveLength(1)
  })

  it('applies edge removal mutation', () => {
    const edge: CanvasEdgeData = {
      id: 'e1',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: 'bezier',
    }
    const state: CanvasStateData = {
      nodes: new Map(),
      edges: [edge],
      hiddenGraphLinks: [],
    }
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      result.current.enqueueEdgesRemove(['e1'])
    })

    expect(result.current.effectiveState.edges).toHaveLength(0)
  })

  it('applies hidden graph link addition mutation', () => {
    const state = makeState([])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))
    const link: HiddenGraphLinkData = {
      subject: '<objects/a>',
      predicate: '<relatedTo>',
      object: '<objects/b>',
      label: 'main',
    }

    act(() => {
      result.current.enqueueHiddenGraphLinksAdd([link])
    })

    expect(result.current.effectiveState.hiddenGraphLinks).toEqual([link])
  })

  it('deduplicates hidden graph links by structured identity', () => {
    const link: HiddenGraphLinkData = {
      subject: '<objects/a>',
      predicate: '<relatedTo>',
      object: '<objects/b>',
    }
    const state = makeState([], [], [link])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      result.current.enqueueHiddenGraphLinksAdd([{ ...link }])
    })

    expect(result.current.effectiveState.hiddenGraphLinks).toHaveLength(1)
  })

  it('applies hidden graph link removal mutation', () => {
    const link: HiddenGraphLinkData = {
      subject: '<objects/a>',
      predicate: '<relatedTo>',
      object: '<objects/b>',
    }
    const state = makeState([], [], [link])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      result.current.enqueueHiddenGraphLinksRemove([{ ...link }])
    })

    expect(result.current.effectiveState.hiddenGraphLinks).toHaveLength(0)
  })

  it('drops confirmed hidden graph link mutation after server state updates', async () => {
    const link: HiddenGraphLinkData = {
      subject: '<objects/a>',
      predicate: '<relatedTo>',
      object: '<objects/b>',
    }
    const send = vi.fn().mockResolvedValue(undefined)
    const { result, rerender } = renderHook(
      ({ serverState }) => useCanvasMutationQueue(serverState, send),
      { initialProps: { serverState: makeState([]) } },
    )

    act(() => {
      result.current.enqueueHiddenGraphLinksAdd([link])
    })
    expect(result.current.pending).toBe(1)

    await act(async () => {
      await Promise.resolve()
    })
    expect(result.current.pending).toBe(1)

    rerender({ serverState: makeState([], [], [link]) })

    expect(result.current.pending).toBe(0)
    expect(result.current.effectiveState.hiddenGraphLinks).toEqual([link])
  })

  it('converges persisted hidden graph links across viewers', async () => {
    const link: HiddenGraphLinkData = {
      subject: '<objects/a>',
      predicate: '<relatedTo>',
      object: '<objects/b>',
    }
    const send = vi.fn().mockResolvedValue(undefined)
    const serverEmpty = makeState([])
    const viewerA = renderHook(
      ({ serverState }) => useCanvasMutationQueue(serverState, send),
      { initialProps: { serverState: serverEmpty } },
    )
    const viewerB = renderHook(
      ({ serverState }) => useCanvasMutationQueue(serverState, send),
      { initialProps: { serverState: serverEmpty } },
    )

    act(() => {
      viewerA.result.current.enqueueHiddenGraphLinksAdd([link])
    })
    expect(viewerA.result.current.effectiveState.hiddenGraphLinks).toEqual([
      link,
    ])
    expect(viewerB.result.current.effectiveState.hiddenGraphLinks).toEqual([])

    await act(async () => {
      await Promise.resolve()
    })
    const serverHidden = makeState([], [], [link])
    viewerA.rerender({ serverState: serverHidden })
    viewerB.rerender({ serverState: serverHidden })

    expect(viewerA.result.current.effectiveState.hiddenGraphLinks).toEqual([
      link,
    ])
    expect(viewerB.result.current.effectiveState.hiddenGraphLinks).toEqual([
      link,
    ])
  })

  it('drops confirmed mutations when server state updates', async () => {
    const state1 = makeState([['a', makeNode({ id: 'a', x: 10 })]])
    let resolveSend: () => void
    const send = vi.fn().mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveSend = resolve
        }),
    )

    const { result, rerender } = renderHook(
      ({ serverState }) => useCanvasMutationQueue(serverState, send),
      { initialProps: { serverState: state1 } },
    )

    // Enqueue a mutation.
    act(() => {
      result.current.enqueueNodesChange(
        new Map([['a', makeNode({ id: 'a', x: 50 })]]),
      )
    })
    expect(result.current.pending).toBe(1)

    // Server confirms (RPC success).
    await act(async () => {
      resolveSend!()
      await Promise.resolve()
    })
    // Still pending until server state updates.
    expect(result.current.pending).toBe(1)

    // Server state updates (streaming).
    const state2 = makeState([['a', makeNode({ id: 'a', x: 50 })]])
    rerender({ serverState: state2 })

    // Now the confirmed mutation should be dropped.
    expect(result.current.pending).toBe(0)
    expect(result.current.effectiveState.nodes.get('a')?.x).toBe(50)
  })

  it('drops a confirmed mutation when server state already updated', async () => {
    const state1 = makeState([['a', makeNode({ id: 'a', x: 10 })]])
    let resolveSend: () => void
    const send = vi.fn().mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveSend = resolve
        }),
    )

    const { result, rerender } = renderHook(
      ({ serverState }) => useCanvasMutationQueue(serverState, send),
      { initialProps: { serverState: state1 } },
    )

    act(() => {
      result.current.enqueueNodesChange(
        new Map([['a', makeNode({ id: 'a', x: 50 })]]),
      )
    })
    expect(result.current.pending).toBe(1)

    rerender({
      serverState: makeState([['a', makeNode({ id: 'a', x: 50 })]]),
    })
    expect(result.current.pending).toBe(1)

    await act(async () => {
      resolveSend!()
      await Promise.resolve()
    })

    expect(result.current.pending).toBe(0)
    expect(result.current.effectiveState.nodes.get('a')?.x).toBe(50)
  })

  it('removes mutation from queue on RPC failure', async () => {
    const state = makeState([['a', makeNode({ id: 'a', x: 10 })]])
    const onError = vi.fn()
    let rejectSend: (err: Error) => void
    const send = vi.fn().mockImplementation(
      () =>
        new Promise<void>((_, reject) => {
          rejectSend = reject
        }),
    )

    const { result } = renderHook(() =>
      useCanvasMutationQueue(state, send, onError),
    )

    act(() => {
      result.current.enqueueNodesChange(
        new Map([['a', makeNode({ id: 'a', x: 50 })]]),
      )
    })
    expect(result.current.pending).toBe(1)
    expect(result.current.effectiveState.nodes.get('a')?.x).toBe(50)

    // RPC fails.
    await act(async () => {
      rejectSend!(new Error('network error'))
      await Promise.resolve()
    })

    // Mutation removed, reverts to server state.
    expect(result.current.pending).toBe(0)
    expect(result.current.effectiveState.nodes.get('a')?.x).toBe(10)
    expect(onError).toHaveBeenCalledOnce()
  })

  it('does not enqueue when sendMutation is null', () => {
    const state = makeState([['a', makeNode({ id: 'a', x: 10 })]])
    const { result } = renderHook(() => useCanvasMutationQueue(state, null))

    act(() => {
      result.current.enqueueNodesChange(
        new Map([['a', makeNode({ id: 'a', x: 50 })]]),
      )
    })

    // Should not enqueue when no send function.
    expect(result.current.pending).toBe(0)
    expect(result.current.effectiveState.nodes.get('a')?.x).toBe(10)
  })

  it('applies multiple mutations in order', () => {
    const state = makeState([
      ['a', makeNode({ id: 'a', x: 0 })],
      ['b', makeNode({ id: 'b', x: 100 })],
    ])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      // Move node a.
      result.current.enqueueNodesChange(
        new Map([['a', makeNode({ id: 'a', x: 50 })]]),
      )
      // Remove node b.
      result.current.enqueueNodesRemove(['b'])
    })

    expect(result.current.pending).toBe(2)
    expect(result.current.effectiveState.nodes.get('a')?.x).toBe(50)
    expect(result.current.effectiveState.nodes.has('b')).toBe(false)
  })

  it('adds new node optimistically', () => {
    const state = makeState([])
    const send = vi.fn().mockResolvedValue(undefined)
    const { result } = renderHook(() => useCanvasMutationQueue(state, send))

    act(() => {
      result.current.enqueueNodesChange(
        new Map([['new', makeNode({ id: 'new', x: 200, y: 300 })]]),
      )
    })

    expect(result.current.effectiveState.nodes.has('new')).toBe(true)
    expect(result.current.effectiveState.nodes.get('new')?.x).toBe(200)
  })
})

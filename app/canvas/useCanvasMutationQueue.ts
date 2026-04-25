import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type {
  CanvasStateData,
  CanvasNodeData,
  CanvasEdgeData,
  HiddenGraphLinkData,
} from './types.js'

// CanvasMutation represents a pending canvas state change.
interface CanvasMutation {
  seq: number
  setNodes?: Map<string, CanvasNodeData>
  removeNodeIds?: string[]
  addEdges?: CanvasEdgeData[]
  removeEdgeIds?: string[]
  addHiddenGraphLinks?: HiddenGraphLinkData[]
  removeHiddenGraphLinks?: HiddenGraphLinkData[]
}

// SendMutationFn sends a mutation to the backend. Resolves on success.
export type SendMutationFn = (mutation: {
  setNodes?: Map<string, CanvasNodeData>
  removeNodeIds?: string[]
  addEdges?: CanvasEdgeData[]
  removeEdgeIds?: string[]
  addHiddenGraphLinks?: HiddenGraphLinkData[]
  removeHiddenGraphLinks?: HiddenGraphLinkData[]
}) => Promise<void>

function graphLinkKey(link: HiddenGraphLinkData): string {
  return `${link.subject}\n${link.predicate}\n${link.object}\n${link.label ?? ''}`
}

// applyMutations applies pending mutations on top of server state.
function applyMutations(
  base: CanvasStateData,
  mutations: CanvasMutation[],
): CanvasStateData {
  if (mutations.length === 0) return base

  const nodes = new Map(base.nodes)
  const edges = [...base.edges]
  const hiddenGraphLinks = [...base.hiddenGraphLinks]

  for (const m of mutations) {
    if (m.setNodes) {
      for (const [id, node] of m.setNodes) {
        nodes.set(id, node)
      }
    }
    if (m.removeNodeIds) {
      for (const id of m.removeNodeIds) {
        nodes.delete(id)
      }
    }
    if (m.addEdges) {
      const existing = new Set(edges.map((e) => e.id))
      for (const edge of m.addEdges) {
        if (!existing.has(edge.id)) {
          edges.push(edge)
        }
      }
    }
    if (m.removeEdgeIds) {
      const remove = new Set(m.removeEdgeIds)
      const indexes = [...edges.keys()].reverse()
      indexes.forEach((i) => {
        if (remove.has(edges[i].id)) {
          edges.splice(i, 1)
        }
      })
    }
    if (m.addHiddenGraphLinks) {
      const existing = new Set(hiddenGraphLinks.map(graphLinkKey))
      for (const link of m.addHiddenGraphLinks) {
        const key = graphLinkKey(link)
        if (!existing.has(key)) {
          hiddenGraphLinks.push(link)
          existing.add(key)
        }
      }
    }
    if (m.removeHiddenGraphLinks) {
      const remove = new Set(m.removeHiddenGraphLinks.map(graphLinkKey))
      const indexes = [...hiddenGraphLinks.keys()].reverse()
      indexes.forEach((i) => {
        if (remove.has(graphLinkKey(hiddenGraphLinks[i]))) {
          hiddenGraphLinks.splice(i, 1)
        }
      })
    }
  }

  return { nodes, edges, hiddenGraphLinks }
}

// MutationQueueResult is the return type of useCanvasMutationQueue.
export interface MutationQueueResult {
  // effectiveState is the server state with pending mutations applied.
  effectiveState: CanvasStateData
  // enqueueNodesChange queues a node set/update mutation.
  enqueueNodesChange: (nodes: Map<string, CanvasNodeData>) => void
  // enqueueNodesRemove queues a node removal mutation.
  enqueueNodesRemove: (nodeIds: string[]) => void
  // enqueueEdgesAdd queues an edge addition mutation.
  enqueueEdgesAdd: (edges: CanvasEdgeData[]) => void
  // enqueueEdgesRemove queues an edge removal mutation.
  enqueueEdgesRemove: (edgeIds: string[]) => void
  // enqueueHiddenGraphLinksAdd queues graph links to hide.
  enqueueHiddenGraphLinksAdd: (links: HiddenGraphLinkData[]) => void
  // enqueueHiddenGraphLinksRemove queues graph links to show again.
  enqueueHiddenGraphLinksRemove: (links: HiddenGraphLinkData[]) => void
  // pending is the number of pending mutations in the queue.
  pending: number
}

// useCanvasMutationQueue manages optimistic canvas state via a mutation queue.
// Mutations are applied locally on top of server state and sent to the backend.
// Once the server confirms (RPC success) and a new streaming state arrives,
// the mutation is dropped from the queue. On RPC failure, the mutation is
// removed immediately (server wins).
export function useCanvasMutationQueue(
  serverState: CanvasStateData | null,
  sendMutation: SendMutationFn | null,
  onError?: (err: unknown) => void,
): MutationQueueResult {
  const nextSeqRef = useRef(0)
  const queueRef = useRef<CanvasMutation[]>([])
  const confirmedSeqs = useRef(new Set<number>())
  const [version, setVersion] = useState(0)

  // When server state updates, drop confirmed mutations.
  useEffect(() => {
    if (!serverState || confirmedSeqs.current.size === 0) return

    const confirmed = confirmedSeqs.current
    const prev = queueRef.current
    const next = prev.filter((m) => !confirmed.has(m.seq))
    if (next.length !== prev.length) {
      queueRef.current = next
      confirmed.clear()
      setVersion((v) => v + 1)
    }
  }, [serverState])

  // Ref for sendMutation so the enqueue callback stays stable.
  const sendRef = useRef(sendMutation)
  sendRef.current = sendMutation

  const enqueue = useCallback(
    (mutation: Omit<CanvasMutation, 'seq'>) => {
      const send = sendRef.current
      if (!send) return

      const seq = nextSeqRef.current++
      const full: CanvasMutation = { ...mutation, seq }
      queueRef.current = [...queueRef.current, full]
      setVersion((v) => v + 1)

      void send(mutation).then(
        () => {
          confirmedSeqs.current.add(seq)
        },
        (err) => {
          // On failure, remove this mutation from queue.
          queueRef.current = queueRef.current.filter((m) => m.seq !== seq)
          setVersion((v) => v + 1)
          onError?.(err)
        },
      )
    },
    [onError],
  )

  const enqueueNodesChange = useCallback(
    (nodes: Map<string, CanvasNodeData>) => {
      enqueue({ setNodes: nodes })
    },
    [enqueue],
  )

  const enqueueNodesRemove = useCallback(
    (nodeIds: string[]) => {
      enqueue({ removeNodeIds: nodeIds })
    },
    [enqueue],
  )

  const enqueueEdgesAdd = useCallback(
    (edges: CanvasEdgeData[]) => {
      enqueue({ addEdges: edges })
    },
    [enqueue],
  )

  const enqueueEdgesRemove = useCallback(
    (edgeIds: string[]) => {
      enqueue({ removeEdgeIds: edgeIds })
    },
    [enqueue],
  )

  const enqueueHiddenGraphLinksAdd = useCallback(
    (links: HiddenGraphLinkData[]) => {
      enqueue({ addHiddenGraphLinks: links })
    },
    [enqueue],
  )

  const enqueueHiddenGraphLinksRemove = useCallback(
    (links: HiddenGraphLinkData[]) => {
      enqueue({ removeHiddenGraphLinks: links })
    },
    [enqueue],
  )

  const base: CanvasStateData = serverState ?? {
    nodes: new Map(),
    edges: [],
    hiddenGraphLinks: [],
  }
  const effectiveState = useMemo(
    () => applyMutations(base, queueRef.current),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [base, version],
  )

  return {
    effectiveState,
    enqueueNodesChange,
    enqueueNodesRemove,
    enqueueEdgesAdd,
    enqueueEdgesRemove,
    enqueueHiddenGraphLinksAdd,
    enqueueHiddenGraphLinksRemove,
    pending: queueRef.current.length,
  }
}

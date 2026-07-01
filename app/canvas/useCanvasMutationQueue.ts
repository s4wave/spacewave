import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type {
  CanvasStateData,
  CanvasNodeData,
  CanvasEdgeData,
  HiddenGraphLinkData,
  CanvasLayoutMetadataData,
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
  setLayoutMetadata?: Map<string, CanvasLayoutMetadataData>
  removeLayoutMetadataNodeIds?: string[]
}

// SendMutationFn sends a mutation to the backend. Resolves on success.
export type SendMutationFn = (mutation: {
  setNodes?: Map<string, CanvasNodeData>
  removeNodeIds?: string[]
  addEdges?: CanvasEdgeData[]
  removeEdgeIds?: string[]
  addHiddenGraphLinks?: HiddenGraphLinkData[]
  removeHiddenGraphLinks?: HiddenGraphLinkData[]
  setLayoutMetadata?: Map<string, CanvasLayoutMetadataData>
  removeLayoutMetadataNodeIds?: string[]
}) => Promise<void>

function graphLinkKey(link: HiddenGraphLinkData): string {
  return `${link.subject}\n${link.predicate}\n${link.object}\n${link.label ?? ''}`
}

function bytesEqual(a?: Uint8Array, b?: Uint8Array): boolean {
  if (!a || a.length === 0) return !b || b.length === 0
  if (!b) return false
  if (a.length !== b.length) return false
  for (const [index, value] of a.entries()) {
    if (value !== b[index]) return false
  }
  return true
}

function nodesEqual(a: CanvasNodeData, b: CanvasNodeData): boolean {
  return (
    a.id === b.id &&
    a.x === b.x &&
    a.y === b.y &&
    a.width === b.width &&
    a.height === b.height &&
    a.zIndex === b.zIndex &&
    a.type === b.type &&
    (a.textContent ?? '') === (b.textContent ?? '') &&
    (a.objectKey ?? '') === (b.objectKey ?? '') &&
    (a.pinned ?? false) === (b.pinned ?? false) &&
    (a.viewPath ?? '') === (b.viewPath ?? '') &&
    bytesEqual(a.shapeData, b.shapeData)
  )
}

function edgesEqual(a: CanvasEdgeData, b: CanvasEdgeData): boolean {
  return (
    a.id === b.id &&
    a.sourceNodeId === b.sourceNodeId &&
    a.targetNodeId === b.targetNodeId &&
    (a.label ?? '') === (b.label ?? '') &&
    a.style === b.style
  )
}

function layoutMetadataEqual(
  a: CanvasLayoutMetadataData,
  b: CanvasLayoutMetadataData,
): boolean {
  return (
    (a.stableNodeId ?? '') === (b.stableNodeId ?? '') &&
    (a.lane ?? '') === (b.lane ?? '') &&
    (a.rank ?? 0) === (b.rank ?? 0) &&
    (a.group ?? '') === (b.group ?? '') &&
    (a.projectionOwner ?? '') === (b.projectionOwner ?? '')
  )
}

function mutationApplied(
  serverState: CanvasStateData | null,
  mutation: CanvasMutation,
): boolean {
  if (!serverState) return false

  if (mutation.setNodes) {
    for (const [id, node] of mutation.setNodes) {
      const serverNode = serverState.nodes.get(id)
      if (!serverNode || !nodesEqual(serverNode, node)) return false
    }
  }
  if (mutation.removeNodeIds) {
    for (const id of mutation.removeNodeIds) {
      if (serverState.nodes.has(id)) return false
    }
  }
  if (mutation.addEdges) {
    const serverEdges = new Map(
      serverState.edges.map((edge) => [edge.id, edge]),
    )
    for (const edge of mutation.addEdges) {
      const serverEdge = serverEdges.get(edge.id)
      if (!serverEdge || !edgesEqual(serverEdge, edge)) return false
    }
  }
  if (mutation.removeEdgeIds) {
    const serverEdgeIds = new Set(serverState.edges.map((edge) => edge.id))
    for (const id of mutation.removeEdgeIds) {
      if (serverEdgeIds.has(id)) return false
    }
  }
  if (mutation.addHiddenGraphLinks) {
    const serverLinks = new Set(serverState.hiddenGraphLinks.map(graphLinkKey))
    for (const link of mutation.addHiddenGraphLinks) {
      if (!serverLinks.has(graphLinkKey(link))) return false
    }
  }
  if (mutation.removeHiddenGraphLinks) {
    const serverLinks = new Set(serverState.hiddenGraphLinks.map(graphLinkKey))
    for (const link of mutation.removeHiddenGraphLinks) {
      if (serverLinks.has(graphLinkKey(link))) return false
    }
  }
  if (mutation.setLayoutMetadata) {
    for (const [id, metadata] of mutation.setLayoutMetadata) {
      const serverMetadata = serverState.layoutMetadata.get(id)
      if (!serverMetadata || !layoutMetadataEqual(serverMetadata, metadata)) {
        return false
      }
    }
  }
  if (mutation.removeLayoutMetadataNodeIds) {
    for (const id of mutation.removeLayoutMetadataNodeIds) {
      if (serverState.layoutMetadata.has(id)) return false
    }
  }

  return true
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
  const layoutMetadata = new Map(base.layoutMetadata)

  for (const m of mutations) {
    if (m.setNodes) {
      for (const [id, node] of m.setNodes) {
        nodes.set(id, node)
      }
    }
    if (m.removeNodeIds) {
      for (const id of m.removeNodeIds) {
        nodes.delete(id)
        layoutMetadata.delete(id)
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
    if (m.setLayoutMetadata) {
      for (const [id, metadata] of m.setLayoutMetadata) {
        layoutMetadata.set(id, metadata)
      }
    }
    if (m.removeLayoutMetadataNodeIds) {
      for (const id of m.removeLayoutMetadataNodeIds) {
        layoutMetadata.delete(id)
      }
    }
  }

  return { nodes, edges, hiddenGraphLinks, layoutMetadata }
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
  // enqueueLayoutMetadataChange queues layout metadata set/update changes.
  enqueueLayoutMetadataChange: (
    layoutMetadata: Map<string, CanvasLayoutMetadataData>,
  ) => void
  // enqueueLayoutMetadataRemove queues layout metadata removal by node ID.
  enqueueLayoutMetadataRemove: (nodeIds: string[]) => void
  // pending is the number of pending mutations in the queue.
  pending: number
}

// useCanvasMutationQueue manages optimistic canvas state via a mutation queue.
// Mutations are applied locally on top of server state and sent to the backend.
// Once the server confirms (RPC success) and watched state contains the change,
// the mutation is dropped from the queue. On RPC failure, the mutation is removed
// immediately (server wins).
export function useCanvasMutationQueue(
  serverState: CanvasStateData | null,
  sendMutation: SendMutationFn | null,
  onError?: (err: unknown) => void,
): MutationQueueResult {
  const nextSeqRef = useRef(0)
  const [queue, setQueue] = useState<CanvasMutation[]>([])
  const confirmedSeqs = useRef(new Set<number>())
  const serverStateRef = useRef(serverState)
  const sendRef = useRef(sendMutation)

  // Confirmed mutations stay optimistic until the watched state contains them.
  useEffect(() => {
    serverStateRef.current = serverState
    if (!serverState || confirmedSeqs.current.size === 0) return

    const confirmed = confirmedSeqs.current
    setQueue((prev) => {
      const next = prev.filter((m) => {
        if (!confirmed.has(m.seq)) return true
        if (!mutationApplied(serverState, m)) return true
        confirmed.delete(m.seq)
        return false
      })
      if (next.length === prev.length) return prev
      return next
    })
  }, [serverState])

  // Ref for sendMutation so the enqueue callback stays stable.
  useEffect(() => {
    sendRef.current = sendMutation
  }, [sendMutation])

  const enqueue = useCallback(
    (mutation: Omit<CanvasMutation, 'seq'>) => {
      const send = sendRef.current
      if (!send) return

      const seq = nextSeqRef.current++
      const full: CanvasMutation = {
        ...mutation,
        seq,
      }
      setQueue((prev) => [...prev, full])

      void send(mutation).then(
        () => {
          confirmedSeqs.current.add(seq)
          setQueue((prev) => {
            const next = prev.filter((m) => {
              if (m.seq !== seq) return true
              if (!mutationApplied(serverStateRef.current, m)) return true
              confirmedSeqs.current.delete(seq)
              return false
            })
            if (next.length === prev.length) return prev
            return next
          })
        },
        (err) => {
          // On failure, remove this mutation from queue.
          setQueue((prev) => prev.filter((m) => m.seq !== seq))
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

  const enqueueLayoutMetadataChange = useCallback(
    (layoutMetadata: Map<string, CanvasLayoutMetadataData>) => {
      enqueue({ setLayoutMetadata: layoutMetadata })
    },
    [enqueue],
  )

  const enqueueLayoutMetadataRemove = useCallback(
    (nodeIds: string[]) => {
      enqueue({ removeLayoutMetadataNodeIds: nodeIds })
    },
    [enqueue],
  )

  const effectiveState = useMemo(() => {
    const base = serverState ?? {
      nodes: new Map(),
      edges: [],
      hiddenGraphLinks: [],
      layoutMetadata: new Map(),
    }
    return applyMutations(base, queue)
  }, [serverState, queue])

  return {
    effectiveState,
    enqueueNodesChange,
    enqueueNodesRemove,
    enqueueEdgesAdd,
    enqueueEdgesRemove,
    enqueueHiddenGraphLinksAdd,
    enqueueHiddenGraphLinksRemove,
    enqueueLayoutMetadataChange,
    enqueueLayoutMetadataRemove,
    pending: queue.length,
  }
}

import { startTransition, useState } from 'react'

import { useAbortSignalEffect } from '@aptre/bldr-react'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

import {
  buildGraphLinkViewModel,
  getSelectedGraphNodes,
} from '../graphLinkViewModel.js'
import type {
  CanvasStateData,
  EphemeralEdge,
  HiddenGraphLinkData,
} from '../types.js'

const graphLinkLookupLimit = 100

interface GraphObjectMetadata {
  label: string
  type?: string
  typeLabel?: string
}

function isAbortError(error: unknown): boolean {
  return (
    error instanceof Error &&
    (error.name === 'AbortError' || error.message === 'ERR_RPC_ABORT')
  )
}

// useCanvasGraphLinks owns the selected-node graph query and its recoverable state.
export function useCanvasGraphLinks({
  canvasState,
  graphLinkObjectMetadata,
  hiddenGraphLinks,
  nodesByObjectKey,
  refreshToken,
  selectedNodeIds,
  worldState,
}: {
  canvasState: CanvasStateData | null
  graphLinkObjectMetadata: Map<string, GraphObjectMetadata>
  hiddenGraphLinks: HiddenGraphLinkData[]
  nodesByObjectKey: Map<string, string>
  refreshToken: number
  selectedNodeIds: Set<string>
  worldState: ObjectViewerComponentProps['worldState']
}) {
  const [state, setState] = useState<{ edges: EphemeralEdge[]; error: string }>(
    {
      edges: [],
      error: '',
    },
  )
  useAbortSignalEffect(
    (signal) => {
      const world = worldState.value
      if (!world || !canvasState || selectedNodeIds.size === 0) {
        startTransition(() => setState({ edges: [], error: '' }))
        return
      }
      void (async () => {
        const selectedNodes = getSelectedGraphNodes(
          selectedNodeIds,
          canvasState.nodes,
        )
        if (!selectedNodes.length) {
          startTransition(() => setState({ edges: [], error: '' }))
          return
        }
        const response = await world.listGraphEdgeBuckets(
          selectedNodes.map((selected) => selected.node.objectKey ?? ''),
          graphLinkLookupLimit,
          { abortSignal: signal },
        )
        if (signal.aborted) return
        const buckets = response.buckets ?? []
        const results = selectedNodes.map((selected, index) => {
          const bucket = buckets[index]
          return {
            selected,
            outgoing: bucket?.outgoing ?? [],
            incoming: bucket?.incoming ?? [],
            outgoingTruncated: bucket?.outgoingTruncated ?? false,
            incomingTruncated: bucket?.incomingTruncated ?? false,
          }
        })
        const edges = buildGraphLinkViewModel(results, nodesByObjectKey, {
          hiddenGraphLinks,
          objectMetadata: graphLinkObjectMetadata,
        })
        if (!signal.aborted)
          startTransition(() => setState({ edges, error: '' }))
      })().catch((error: unknown) => {
        if (signal.aborted || isAbortError(error)) return
        startTransition(() =>
          setState({
            edges: [],
            error:
              'Canvas graph links could not load. Select the node to retry.',
          }),
        )
      })
    },
    [
      canvasState,
      graphLinkObjectMetadata,
      hiddenGraphLinks,
      nodesByObjectKey,
      refreshToken,
      selectedNodeIds,
      worldState.value,
    ],
  )
  return state
}

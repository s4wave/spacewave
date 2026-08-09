import { useCallback, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { CanvasHandle } from '@s4wave/sdk/canvas/canvas.js'
import {
  EdgeStyle,
  type CanvasEdge as ProtoCanvasEdge,
  type CanvasNode as ProtoCanvasNode,
} from '@s4wave/sdk/canvas/canvas.pb.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'

import { canvasNodeToProto } from '../canvas-node-proto.js'
import {
  useCanvasMutationQueue,
  type SendMutationFn,
} from '../useCanvasMutationQueue.js'
import {
  canvasStateFromProto,
  hiddenGraphLinkToProto,
  layoutMetadataToProto,
} from './canvas-state-codec.js'

// useCanvasResourceController owns the canvas handle, state stream, and mutation projection.
export function useCanvasResourceController(
  worldState: ObjectViewerComponentProps['worldState'],
  objectKey: string,
  onMutationError: () => void,
) {
  const canvasResource = useAccessTypedHandle(
    worldState,
    objectKey,
    CanvasHandle,
  )
  const canvasStateResource = useStreamingResource(
    canvasResource,
    (handle, signal) => handle.watchState(signal),
    [],
  )
  const canvasState = useMemo(
    () =>
      canvasStateResource.value
        ? canvasStateFromProto(
            canvasStateResource.value.nodes ?? {},
            canvasStateResource.value.edges ?? [],
            canvasStateResource.value.hiddenGraphLinks ?? [],
            canvasStateResource.value.layoutMetadata ?? {},
          )
        : null,
    [canvasStateResource.value],
  )
  const sendMutation = useCallback<SendMutationFn>(
    async (mutation) => {
      const handle = canvasResource.value
      if (!handle) throw new Error('no canvas handle')
      const setNodes: Record<string, ProtoCanvasNode> | undefined =
        mutation.setNodes
          ? Object.fromEntries(
              [...mutation.setNodes].map(([id, node]) => [
                id,
                canvasNodeToProto(node),
              ]),
            )
          : undefined
      const addEdges: ProtoCanvasEdge[] | undefined = mutation.addEdges?.map(
        (edge) => ({
          id: edge.id,
          sourceNodeId: edge.sourceNodeId,
          targetNodeId: edge.targetNodeId,
          label: edge.label ?? '',
          style:
            edge.style === 'straight' ? EdgeStyle.STRAIGHT : EdgeStyle.BEZIER,
        }),
      )
      await handle.update({
        setNodes,
        removeNodeIds: mutation.removeNodeIds,
        addEdges,
        removeEdgeIds: mutation.removeEdgeIds,
        addHiddenGraphLinks: mutation.addHiddenGraphLinks?.map(
          hiddenGraphLinkToProto,
        ),
        removeHiddenGraphLinks: mutation.removeHiddenGraphLinks?.map(
          hiddenGraphLinkToProto,
        ),
        setLayoutMetadata: mutation.setLayoutMetadata
          ? Object.fromEntries(
              [...mutation.setLayoutMetadata].map(([id, metadata]) => [
                id,
                layoutMetadataToProto(metadata),
              ]),
            )
          : undefined,
        removeLayoutMetadataNodeIds: mutation.removeLayoutMetadataNodeIds,
      })
    },
    [canvasResource.value],
  )
  const mutations = useCanvasMutationQueue(
    canvasState,
    canvasResource.value ? sendMutation : null,
    onMutationError,
  )
  return {
    canvasState,
    loading: canvasStateResource.loading,
    error: canvasStateResource.error,
    retry: canvasStateResource.retry,
    ...mutations,
  }
}

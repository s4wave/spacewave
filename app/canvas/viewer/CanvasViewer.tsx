/* eslint-disable react-doctor/no-giant-component, react-doctor/rerender-state-only-in-handlers */
import { startTransition, useCallback, useMemo, useState } from 'react'

import { useAbortSignalEffect } from '@aptre/bldr-react'
import { CanvasHandle } from '@s4wave/sdk/canvas/canvas.js'
import {
  NodeType,
  EdgeStyle,
  type CanvasNode as ProtoCanvasNode,
  type CanvasEdge as ProtoCanvasEdge,
  type HiddenGraphLink as ProtoHiddenGraphLink,
  type CanvasLayoutMetadata as ProtoCanvasLayoutMetadata,
} from '@s4wave/sdk/canvas/canvas.pb.js'

import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { getObjectTypeLabel } from '@s4wave/web/space/object-tree.js'
import { useUnixFSRootHandle } from '@s4wave/web/hooks/useUnixFSHandle.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'

import { Canvas } from '../Canvas.js'
import type {
  CanvasStateData,
  CanvasNodeData,
  CanvasEdgeData,
  HiddenGraphLinkData,
  CanvasLayoutMetadataData,
  CanvasCallbacks,
  EphemeralEdge,
  NodeType as CanvasNodeType,
  EdgeStyle as CanvasEdgeStyle,
} from '../types.js'
import {
  useCanvasMutationQueue,
  type SendMutationFn,
} from '../useCanvasMutationQueue.js'
import {
  buildGraphLinkViewModel,
  getSelectedGraphNodes,
} from '../graphLinkViewModel.js'
import { CanvasObjectNode } from './CanvasObjectNode.js'
import { deleteCanvasGraphLink } from './graphLinkActions.js'
import { isCanvasInsertableObject } from './object-picker.js'
import { CanvasTypeID } from '../type.js'
import { getUnixFSImageSubItems } from '../unixfs-image-sub-items.js'

export { CanvasTypeID }

const graphLinkLookupLimit = 100

function isAbortError(err: unknown): boolean {
  if (!(err instanceof Error)) return false
  return err.name === 'AbortError' || err.message === 'ERR_RPC_ABORT'
}

// protoNodeTypeToCanvas converts proto NodeType to canvas NodeType.
function protoNodeTypeToCanvas(t: NodeType): CanvasNodeType {
  switch (t) {
    case NodeType.TEXT:
      return 'text'
    case NodeType.SHAPE:
      return 'shape'
    case NodeType.WORLD_OBJECT:
      return 'world_object'
    case NodeType.DRAWING:
      return 'drawing'
    default:
      return 'text'
  }
}

// canvasNodeTypeToProto converts canvas NodeType to proto NodeType.
function canvasNodeTypeToProto(t: CanvasNodeType): NodeType {
  switch (t) {
    case 'text':
      return NodeType.TEXT
    case 'shape':
      return NodeType.SHAPE
    case 'world_object':
      return NodeType.WORLD_OBJECT
    case 'drawing':
      return NodeType.DRAWING
  }
}

// protoEdgeStyleToCanvas converts proto EdgeStyle to canvas EdgeStyle.
function protoEdgeStyleToCanvas(s: EdgeStyle): CanvasEdgeStyle {
  switch (s) {
    case EdgeStyle.STRAIGHT:
      return 'straight'
    default:
      return 'bezier'
  }
}

// protoToCanvasState converts proto CanvasState to the canvas module's data types.
function protoToCanvasState(
  nodes: Record<string, ProtoCanvasNode>,
  edges: ProtoCanvasEdge[],
  hiddenGraphLinks: ProtoHiddenGraphLink[],
  layoutMetadata: Record<string, ProtoCanvasLayoutMetadata>,
): CanvasStateData {
  const nodeMap = new Map<string, CanvasNodeData>()
  for (const [id, n] of Object.entries(nodes)) {
    nodeMap.set(id, {
      id,
      x: n.x ?? 0,
      y: n.y ?? 0,
      width: n.width ?? 200,
      height: n.height ?? 150,
      zIndex: n.zIndex ?? 0,
      type: protoNodeTypeToCanvas(n.type ?? NodeType.UNKNOWN),
      textContent: n.textContent || undefined,
      shapeData: n.shapeData?.length ? n.shapeData : undefined,
      objectKey: n.objectKey || undefined,
      pinned: n.pinned,
      viewPath: n.viewPath || undefined,
    })
  }
  const edgeList: CanvasEdgeData[] = edges.map((e) => ({
    id: e.id ?? '',
    sourceNodeId: e.sourceNodeId ?? '',
    targetNodeId: e.targetNodeId ?? '',
    label: e.label || undefined,
    style: protoEdgeStyleToCanvas(e.style ?? EdgeStyle.BEZIER),
  }))
  const hiddenLinkList: HiddenGraphLinkData[] = hiddenGraphLinks.map(
    (link) => ({
      subject: link.subject ?? '',
      predicate: link.predicate ?? '',
      object: link.object ?? '',
      label: link.label || undefined,
    }),
  )
  const layoutMetadataMap = new Map<string, CanvasLayoutMetadataData>()
  for (const [id, metadata] of Object.entries(layoutMetadata)) {
    layoutMetadataMap.set(id, {
      stableNodeId: metadata.stableNodeId || undefined,
      lane: metadata.lane || undefined,
      rank: metadata.rank ?? undefined,
      group: metadata.group || undefined,
      projectionOwner: metadata.projectionOwner || undefined,
    })
  }
  return {
    nodes: nodeMap,
    edges: edgeList,
    hiddenGraphLinks: hiddenLinkList,
    layoutMetadata: layoutMetadataMap,
  }
}

// canvasNodeToProto converts a canvas node to proto format.
function canvasNodeToProto(n: CanvasNodeData): ProtoCanvasNode {
  return {
    id: n.id,
    x: n.x,
    y: n.y,
    width: n.width,
    height: n.height,
    zIndex: n.zIndex,
    type: canvasNodeTypeToProto(n.type),
    textContent: n.textContent ?? '',
    shapeData: n.shapeData ?? new Uint8Array(),
    objectKey: n.objectKey ?? '',
    pinned: n.pinned ?? false,
    viewPath: n.viewPath ?? '',
  }
}

// hiddenGraphLinkToProto converts a hidden graph link to proto format.
function hiddenGraphLinkToProto(
  link: HiddenGraphLinkData,
): ProtoHiddenGraphLink {
  return {
    subject: link.subject,
    predicate: link.predicate,
    object: link.object,
    label: link.label ?? '',
  }
}

// layoutMetadataToProto converts layout metadata to proto format.
function layoutMetadataToProto(
  metadata: CanvasLayoutMetadataData,
): ProtoCanvasLayoutMetadata {
  return {
    stableNodeId: metadata.stableNodeId ?? '',
    lane: metadata.lane ?? '',
    rank: metadata.rank ?? 0,
    group: metadata.group ?? '',
    projectionOwner: metadata.projectionOwner ?? '',
  }
}

// CanvasViewer is the ObjectType viewer for canvas objects.
export function CanvasViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const spaceContainer = SpaceContainerContext.useContextSafe()
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set())
  const [focusNodeId, setFocusNodeId] = useState<string | null>(null)
  const [graphLinkRefreshTick, setGraphLinkRefreshTick] = useState(0)
  const [graphLinkActionError, setGraphLinkActionError] = useState<
    string | null
  >(null)

  const unixfsObjectKey =
    spaceContainer?.spaceState.worldContents?.objects?.find(
      (object) => object.objectType === UnixFSTypeID,
    )?.objectKey ?? null
  const unixfsRoot = useUnixFSRootHandle(worldState, unixfsObjectKey)
  const imageSubItems: SubItemsCallback = useCallback(
    (query, signal) =>
      unixfsRoot.value
        ? getUnixFSImageSubItems(unixfsRoot.value, query, signal)
        : Promise.resolve([]),
    [unixfsRoot.value],
  )
  // Access the SRPC resource for this canvas object.
  const canvasResource = useAccessTypedHandle(
    worldState,
    objectKey,
    CanvasHandle,
  )

  // Watch canvas state via streaming RPC.
  const canvasStateResource = useStreamingResource(
    canvasResource,
    (handle, signal) => handle.watchState(signal),
    [],
  )

  const canvasStateData = useMemo(
    () =>
      canvasStateResource.value
        ? protoToCanvasState(
            canvasStateResource.value.nodes ?? {},
            canvasStateResource.value.edges ?? [],
            canvasStateResource.value.hiddenGraphLinks ?? [],
            canvasStateResource.value.layoutMetadata ?? {},
          )
        : null,
    [canvasStateResource.value],
  )

  // Pre-index canvas nodes by objectKey for O(1) lookup.
  const nodesByObjectKey = useMemo(() => {
    if (!canvasStateData) return new Map<string, string>()
    const m = new Map<string, string>()
    for (const [nid, n] of canvasStateData.nodes) {
      if (n.objectKey) m.set(n.objectKey, nid)
    }
    return m
  }, [canvasStateData])

  const graphLinkObjectMetadata = useMemo(() => {
    const m = new Map<
      string,
      { label: string; type?: string; typeLabel?: string }
    >()
    for (const obj of spaceContainer?.spaceState.worldContents?.objects ?? []) {
      const key = obj.objectKey ?? ''
      if (!key) continue
      const type = obj.objectType ?? ''
      m.set(key, {
        label: key,
        type: type || undefined,
        typeLabel: type ? getObjectTypeLabel(type) : undefined,
      })
    }
    return m
  }, [spaceContainer?.spaceState.worldContents?.objects])

  const sendMutation = useCallback<SendMutationFn>(
    async (mutation) => {
      const handle = canvasResource.value
      if (!handle) throw new Error('no canvas handle')

      const setNodes: Record<string, ProtoCanvasNode> | undefined =
        mutation.setNodes
          ? Object.fromEntries(
              [...mutation.setNodes].map(([id, n]) => [
                id,
                canvasNodeToProto(n),
              ]),
            )
          : undefined

      const addEdges: ProtoCanvasEdge[] | undefined = mutation.addEdges?.map(
        (e) => ({
          id: e.id,
          sourceNodeId: e.sourceNodeId,
          targetNodeId: e.targetNodeId,
          label: e.label ?? '',
          style: e.style === 'straight' ? EdgeStyle.STRAIGHT : EdgeStyle.BEZIER,
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

  const {
    effectiveState,
    enqueueNodesChange,
    enqueueNodesRemove,
    enqueueEdgesAdd,
    enqueueHiddenGraphLinksAdd,
    pending,
  } = useCanvasMutationQueue(
    canvasStateData,
    canvasResource.value ? sendMutation : null,
    () => {
      setGraphLinkActionError(
        'Canvas graph-link update failed. The optimistic change was rolled back.',
      )
    },
  )

  // Query ephemeral edges for selected world_object nodes.
  const [ephemeralEdges, setEphemeralEdges] = useState<EphemeralEdge[]>([])
  useAbortSignalEffect(
    (signal) => {
      const world = worldState.value
      if (!world || !canvasStateData || selectedNodeIds.size === 0) {
        startTransition(() => {
          setEphemeralEdges([])
        })
        return
      }
      void (async () => {
        const selectedNodes = getSelectedGraphNodes(
          selectedNodeIds,
          canvasStateData.nodes,
        )
        if (selectedNodes.length === 0) {
          startTransition(() => {
            setEphemeralEdges([])
          })
          return
        }

        // eslint-disable-next-line react-doctor/async-defer-await
        const bucketResponse = await world.listGraphEdgeBuckets(
          selectedNodes.map((selected) => selected.node.objectKey ?? ''),
          graphLinkLookupLimit,
          { abortSignal: signal },
        )
        if (signal.aborted) return
        const buckets = bucketResponse.buckets ?? []
        const perNodeResults = selectedNodes.map((selected, index) => {
          const bucket = buckets[index]
          return {
            selected,
            outgoing: bucket.outgoing ?? [],
            incoming: bucket.incoming ?? [],
            outgoingTruncated: bucket.outgoingTruncated ?? false,
            incomingTruncated: bucket.incomingTruncated ?? false,
          }
        })

        const edges = buildGraphLinkViewModel(
          perNodeResults,
          nodesByObjectKey,
          {
            hiddenGraphLinks: effectiveState.hiddenGraphLinks,
            objectMetadata: graphLinkObjectMetadata,
          },
        )
        if (!signal.aborted) {
          startTransition(() => {
            setEphemeralEdges(edges)
          })
        }
      })().catch((err: unknown) => {
        if (signal.aborted || isAbortError(err)) return
        throw err
      })
    },
    [
      worldState.value,
      canvasStateData,
      effectiveState.hiddenGraphLinks,
      selectedNodeIds,
      nodesByObjectKey,
      graphLinkObjectMetadata,
      graphLinkRefreshTick,
    ],
  )

  const handlePinObject = useCallback(
    (linkedObjectKey: string, x: number, y: number) => {
      setGraphLinkActionError(null)
      const nodeId = `node-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
      enqueueNodesChange(
        new Map([
          [
            nodeId,
            {
              id: nodeId,
              x,
              y,
              width: 400,
              height: 300,
              zIndex: 0,
              type: 'world_object' as const,
              objectKey: linkedObjectKey,
              pinned: true,
            },
          ],
        ]),
      )
    },
    [enqueueNodesChange],
  )

  const handleNodeSelect = useCallback((nodeIds: Set<string>) => {
    setSelectedNodeIds(nodeIds)
  }, [])

  const handleFocusObject = useCallback(
    (_objectKey: string, nodeId: string) => {
      if (!effectiveState.nodes.has(nodeId)) {
        setGraphLinkActionError(
          'Cannot focus graph target because it is no longer on this canvas.',
        )
        return
      }
      setGraphLinkActionError(null)
      setFocusNodeId(nodeId)
    },
    [effectiveState.nodes],
  )

  const handleHideGraphLink = useCallback(
    (link: EphemeralEdge) => {
      setGraphLinkActionError(null)
      enqueueHiddenGraphLinksAdd([
        {
          subject: link.subject,
          predicate: link.predicate,
          object: link.object,
          label: link.label,
        },
      ])
    },
    [enqueueHiddenGraphLinksAdd],
  )

  const handleDeleteGraphLink = useCallback(
    (link: EphemeralEdge) => {
      setGraphLinkActionError(null)
      void deleteCanvasGraphLink({
        link,
        world: worldState.value,
        onError: setGraphLinkActionError,
        onDeleted: () => {
          startTransition(() => {
            setGraphLinkRefreshTick((v) => v + 1)
          })
        },
      })
    },
    [worldState.value],
  )

  const objectSubItems: SubItemsCallback | undefined = useCallback(
    (query: string) => {
      const objects = spaceContainer?.spaceState.worldContents?.objects ?? []
      const q = query.toLowerCase()

      return Promise.resolve(
        objects.flatMap((obj) => {
          const key = obj.objectKey ?? ''
          const type = obj.objectType ?? ''
          const label = getObjectTypeLabel(type).toLowerCase()
          if (
            !isCanvasInsertableObject(key, type, objectKey) ||
            (q && !key.toLowerCase().includes(q) && !label.includes(q))
          ) {
            return []
          }
          return [
            {
              id: key,
              label: key,
              description: getObjectTypeLabel(type),
            },
          ]
        }),
      )
    },
    [spaceContainer, objectKey],
  )

  // Handle view path changes within embedded object nodes.
  const handleViewPathChange = useCallback(
    (nodeId: string, node: CanvasNodeData, path: string) => {
      if (path === (node.viewPath || '/')) return
      enqueueNodesChange(new Map([[nodeId, { ...node, viewPath: path }]]))
    },
    [enqueueNodesChange],
  )

  // Render world_object nodes via CanvasObjectNode.
  const renderNodeContent = useCallback(
    (node: CanvasNodeData) => {
      if (node.type !== 'world_object' || !node.objectKey) return null

      // Block self-embedding.
      if (node.objectKey === objectKey) {
        return (
          <div className="text-muted-foreground flex h-full items-center justify-center text-sm">
            Cannot embed canvas within itself
          </div>
        )
      }

      return (
        <CanvasObjectNode
          objectKey={node.objectKey}
          canvasObjectKey={objectKey}
          nodeId={node.id}
          worldState={worldState}
          viewPath={node.viewPath}
          onViewPathChange={(path) => handleViewPathChange(node.id, node, path)}
        />
      )
    },
    [objectKey, worldState, handleViewPathChange],
  )

  const callbacks: CanvasCallbacks = useMemo(
    () => ({
      onNodesChange: enqueueNodesChange,
      onNodesRemove: enqueueNodesRemove,
      onEdgesChange: enqueueEdgesAdd,
      onNodeSelect: handleNodeSelect,
      onPinObject: handlePinObject,
      onFocusObject: handleFocusObject,
      onHideGraphLink: handleHideGraphLink,
      onDeleteGraphLink: handleDeleteGraphLink,
      renderNodeContent,
    }),
    [
      enqueueNodesChange,
      enqueueNodesRemove,
      enqueueEdgesAdd,
      handleNodeSelect,
      handlePinObject,
      handleFocusObject,
      handleHideGraphLink,
      handleDeleteGraphLink,
      renderNodeContent,
    ],
  )

  if (canvasResource.loading || !canvasStateData) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center">
        Loading canvas…
      </div>
    )
  }

  return (
    <div className="relative h-full w-full">
      {graphLinkActionError && (
        <div className="border-destructive/20 bg-background-card/90 text-destructive pointer-events-none absolute top-3 left-1/2 z-20 -translate-x-1/2 rounded-md border px-3 py-2 text-xs shadow-lg backdrop-blur-sm">
          {graphLinkActionError}
        </div>
      )}
      <Canvas
        state={effectiveState}
        ephemeralEdges={ephemeralEdges.length > 0 ? ephemeralEdges : undefined}
        callbacks={callbacks}
        pendingMutations={pending}
        objectSubItems={spaceContainer ? objectSubItems : undefined}
        imageObjectKey={
          unixfsRoot.value ? (unixfsObjectKey ?? undefined) : undefined
        }
        imageSubItems={unixfsRoot.value ? imageSubItems : undefined}
        focusNodeId={focusNodeId}
        className="h-full w-full"
      />
    </div>
  )
}

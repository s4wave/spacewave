import { startTransition, useCallback, useMemo, useState } from 'react'

import type { SubItemsCallback } from '@s4wave/web/command/CommandContext.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useUnixFSRootHandle } from '@s4wave/web/hooks/useUnixFSHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { getObjectTypeLabel } from '@s4wave/web/space/object-tree.js'
import { UnixFSTypeID } from '@s4wave/sdk/unixfs/type.js'

import { Canvas } from '../Canvas.js'
import type {
  CanvasCallbacks,
  CanvasNodeData,
  EphemeralEdge,
} from '../types.js'
import { CanvasTypeID } from '../type.js'
import { getUnixFSImageSubItems } from '../unixfs-image-sub-items.js'
import { CanvasObjectNode } from './CanvasObjectNode.js'
import { protoEdgeStyleToCanvas } from './canvas-state-codec.js'
import { deleteCanvasGraphLink } from './graphLinkActions.js'
import { isCanvasInsertableObject } from './object-picker.js'
import { useCanvasGraphLinks } from './useCanvasGraphLinks.js'
import { useCanvasResourceController } from './useCanvasResourceController.js'

export { CanvasTypeID, protoEdgeStyleToCanvas }

// CanvasViewer composes the canvas stream, graph query, and rendering owners.
export function CanvasViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const spaceContainer = SpaceContainerContext.useContextSafe()
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set())
  const [focusNodeId, setFocusNodeId] = useState<string | null>(null)
  const [graphLinkRefreshTick, setGraphLinkRefreshTick] = useState(0)
  const [actionError, setActionError] = useState('')
  const resource = useCanvasResourceController(worldState, objectKey, () => {
    setActionError(
      'Canvas graph-link update failed. The optimistic change was rolled back.',
    )
  })
  const {
    effectiveState,
    enqueueEdgesAdd,
    enqueueHiddenGraphLinksAdd,
    enqueueNodesChange,
    enqueueNodesRemove,
  } = resource
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
  const nodesByObjectKey = useMemo(() => {
    const nodes = new Map<string, string>()
    for (const [nodeId, node] of resource.canvasState?.nodes ?? []) {
      if (node.objectKey) nodes.set(node.objectKey, nodeId)
    }
    return nodes
  }, [resource.canvasState])
  const graphLinkObjectMetadata = useMemo(() => {
    const metadata = new Map<
      string,
      { label: string; type?: string; typeLabel?: string }
    >()
    for (const object of spaceContainer?.spaceState.worldContents?.objects ??
      []) {
      const key = object.objectKey ?? ''
      if (!key) continue
      const type = object.objectType ?? ''
      metadata.set(key, {
        label: key,
        type: type || undefined,
        typeLabel: type ? getObjectTypeLabel(type) : undefined,
      })
    }
    return metadata
  }, [spaceContainer?.spaceState.worldContents?.objects])
  const graphLinks = useCanvasGraphLinks({
    canvasState: resource.canvasState,
    graphLinkObjectMetadata,
    hiddenGraphLinks: effectiveState.hiddenGraphLinks,
    nodesByObjectKey,
    refreshToken: graphLinkRefreshTick,
    selectedNodeIds,
    worldState,
  })
  const handlePinObject = useCallback(
    (linkedObjectKey: string, x: number, y: number) => {
      setActionError('')
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
  const handleFocusObject = useCallback(
    (_objectKey: string, nodeId: string) => {
      if (!effectiveState.nodes.has(nodeId)) {
        setActionError(
          'Cannot focus graph target because it is no longer on this canvas.',
        )
        return
      }
      setActionError('')
      setFocusNodeId(nodeId)
    },
    [effectiveState.nodes],
  )
  const handleHideGraphLink = useCallback(
    (link: EphemeralEdge) => {
      setActionError('')
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
      setActionError('')
      void deleteCanvasGraphLink({
        link,
        world: worldState.value,
        onError: setActionError,
        onDeleted: () =>
          startTransition(() => setGraphLinkRefreshTick((value) => value + 1)),
      })
    },
    [worldState.value],
  )
  const objectSubItems: SubItemsCallback = useCallback(
    (query: string) => {
      const normalizedQuery = query.toLowerCase()
      return Promise.resolve(
        (spaceContainer?.spaceState.worldContents?.objects ?? []).flatMap(
          (object) => {
            const key = object.objectKey ?? ''
            const type = object.objectType ?? ''
            const label = getObjectTypeLabel(type).toLowerCase()
            if (
              !isCanvasInsertableObject(key, type, objectKey) ||
              (normalizedQuery &&
                !key.toLowerCase().includes(normalizedQuery) &&
                !label.includes(normalizedQuery))
            )
              return []
            return [
              { id: key, label: key, description: getObjectTypeLabel(type) },
            ]
          },
        ),
      )
    },
    [objectKey, spaceContainer?.spaceState.worldContents?.objects],
  )
  const handleViewPathChange = useCallback(
    (nodeId: string, node: CanvasNodeData, path: string) => {
      if (path === (node.viewPath || '/')) return
      enqueueNodesChange(new Map([[nodeId, { ...node, viewPath: path }]]))
    },
    [enqueueNodesChange],
  )
  const renderNodeContent = useCallback(
    (node: CanvasNodeData) => {
      if (node.type !== 'world_object' || !node.objectKey) return null
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
    [handleViewPathChange, objectKey, worldState],
  )
  const callbacks: CanvasCallbacks = useMemo(
    () => ({
      onNodesChange: enqueueNodesChange,
      onNodesRemove: enqueueNodesRemove,
      onEdgesChange: enqueueEdgesAdd,
      onNodeSelect: setSelectedNodeIds,
      onPinObject: handlePinObject,
      onFocusObject: handleFocusObject,
      onHideGraphLink: handleHideGraphLink,
      onDeleteGraphLink: handleDeleteGraphLink,
      renderNodeContent,
    }),
    [
      handleDeleteGraphLink,
      handleFocusObject,
      handleHideGraphLink,
      handlePinObject,
      renderNodeContent,
      enqueueEdgesAdd,
      enqueueNodesChange,
      enqueueNodesRemove,
    ],
  )
  if (resource.error && !resource.canvasState) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
        <div className="text-destructive text-sm" role="alert">
          Canvas could not be loaded. {resource.error.message}
        </div>
        <button
          type="button"
          onClick={resource.retry}
          className="border-foreground/15 bg-background-card hover:bg-foreground/5 rounded-md border px-3 py-1.5 text-xs"
        >
          Retry
        </button>
      </div>
    )
  }
  if (resource.loading || !resource.canvasState) {
    return (
      <div className="text-muted-foreground flex h-full items-center justify-center">
        Loading canvas…
      </div>
    )
  }
  const error =
    actionError ||
    graphLinks.error ||
    (resource.error ? `Canvas updates stopped. ${resource.error.message}` : '')
  return (
    <div className="relative h-full w-full">
      {error && (
        <div
          role="alert"
          className="border-destructive/20 bg-background-card/90 text-destructive absolute top-3 left-1/2 z-20 -translate-x-1/2 rounded-md border px-3 py-2 text-xs shadow-lg backdrop-blur-sm"
        >
          {error}
          {resource.error && (
            <button
              type="button"
              onClick={resource.retry}
              className="ml-2 underline underline-offset-2"
            >
              Retry
            </button>
          )}
        </div>
      )}
      <Canvas
        state={effectiveState}
        ephemeralEdges={graphLinks.edges.length ? graphLinks.edges : undefined}
        callbacks={callbacks}
        pendingMutations={resource.pending}
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

import { useMemo, useCallback } from 'react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { resolvePath, type To } from '@s4wave/web/router/router.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import { pluginPathPrefix } from '@s4wave/app/urls.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'

// CanvasObjectNodeProps are the props for CanvasObjectNode.
interface CanvasObjectNodeProps {
  objectKey: string
  canvasObjectKey: string
  nodeId: string
  worldState: Resource<IWorldState>
  viewPath?: string
  onViewPathChange?: (path: string) => void
}

// CanvasObjectNode renders a full interactive ObjectViewer for a world object
// embedded in a canvas node. Uses standalone mode with its own bottom bar.
export function CanvasObjectNode({
  objectKey,
  canvasObjectKey,
  nodeId,
  worldState,
  viewPath,
  onViewPathChange,
}: CanvasObjectNodeProps) {
  const spaceCtx = SpaceContainerContext.useContextSafe()
  const sessionIndex = useSessionIndex()
  const currentPath = viewPath || '/'

  const exportUrl = useMemo(
    () =>
      sessionIndex != null && spaceCtx?.spaceId ?
        `${pluginPathPrefix}/export/u/${sessionIndex}/so/${encodeURIComponent(spaceCtx.spaceId)}`
      : undefined,
    [sessionIndex, spaceCtx?.spaceId],
  )

  const handleViewerNavigate = useCallback(
    (to: To) => {
      const resolved = resolvePath(currentPath, to)
      onViewPathChange?.(resolved)
    },
    [currentPath, onViewPathChange],
  )

  const objectInfo: ObjectInfo = useMemo(
    () => ({
      info: {
        case: 'worldObjectInfo' as const,
        value: { objectKey },
      },
    }),
    [objectKey],
  )

  const stateNamespace = useMemo(
    () => ['canvas', canvasObjectKey, 'node', nodeId, 'viewer'],
    [canvasObjectKey, nodeId],
  )

  return (
    <ObjectViewer
      objectInfo={objectInfo}
      worldState={worldState}
      standalone
      bottomBarId={`canvas-node-${nodeId}`}
      path={currentPath}
      exportUrl={exportUrl}
      onNavigate={handleViewerNavigate}
      stateNamespace={stateNamespace}
    />
  )
}

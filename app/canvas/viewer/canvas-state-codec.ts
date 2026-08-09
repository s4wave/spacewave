import {
  EdgeStyle,
  type CanvasEdge as ProtoCanvasEdge,
  type CanvasLayoutMetadata as ProtoCanvasLayoutMetadata,
  type CanvasNode as ProtoCanvasNode,
  type HiddenGraphLink as ProtoHiddenGraphLink,
} from '@s4wave/sdk/canvas/canvas.pb.js'

import { canvasNodeFromPersistedProto } from '../canvas-node-proto.js'
import type {
  CanvasEdgeData,
  CanvasLayoutMetadataData,
  CanvasNodeData,
  CanvasStateData,
  EdgeStyle as CanvasEdgeStyle,
  HiddenGraphLinkData,
} from '../types.js'

// protoEdgeStyleToCanvas converts the persisted enum into a canvas edge style.
export function protoEdgeStyleToCanvas(style: EdgeStyle): CanvasEdgeStyle {
  return style === EdgeStyle.STRAIGHT ? 'straight' : 'bezier'
}

export function canvasStateFromProto(
  nodes: Record<string, ProtoCanvasNode>,
  edges: ProtoCanvasEdge[],
  hiddenGraphLinks: ProtoHiddenGraphLink[],
  layoutMetadata: Record<string, ProtoCanvasLayoutMetadata>,
): CanvasStateData {
  const nodeMap = new Map<string, CanvasNodeData>()
  for (const [id, node] of Object.entries(nodes)) {
    nodeMap.set(id, canvasNodeFromPersistedProto(node, id))
  }
  const edgeList: CanvasEdgeData[] = edges.map((edge) => ({
    id: edge.id ?? '',
    sourceNodeId: edge.sourceNodeId ?? '',
    targetNodeId: edge.targetNodeId ?? '',
    label: edge.label || undefined,
    style: protoEdgeStyleToCanvas(edge.style ?? EdgeStyle.BEZIER),
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

export function hiddenGraphLinkToProto(
  link: HiddenGraphLinkData,
): ProtoHiddenGraphLink {
  return {
    subject: link.subject,
    predicate: link.predicate,
    object: link.object,
    label: link.label ?? '',
  }
}

export function layoutMetadataToProto(
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

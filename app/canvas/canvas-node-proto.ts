import {
  CanvasGeometryKind,
  NodeType,
  type CanvasNode as ProtoCanvasNode,
} from '@s4wave/sdk/canvas/canvas.pb.js'

import {
  canvasGeometryFromProto,
  canvasGeometryToProto,
  decodeLegacyCanvasGeometry,
  encodeLegacyCanvasGeometry,
} from './geometry.js'
import type { CanvasNodeData, NodeType as CanvasNodeType } from './types.js'

function nodeTypeFromProto(type: NodeType | undefined): CanvasNodeType | null {
  switch (type) {
    case NodeType.TEXT:
      return 'text'
    case NodeType.SHAPE:
      return 'shape'
    case NodeType.WORLD_OBJECT:
      return 'world_object'
    case NodeType.DRAWING:
      return 'drawing'
    default:
      return null
  }
}

function nodeTypeToProto(type: CanvasNodeType): NodeType {
  switch (type) {
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

function parseCanvasNode(
  node: ProtoCanvasNode,
  mapKey: string | undefined,
  strict: boolean,
): CanvasNodeData | null {
  const id = mapKey ?? node.id ?? ''
  const type = nodeTypeFromProto(node.type) ?? (strict ? null : 'text')
  const x = node.x ?? 0
  const y = node.y ?? 0
  const width = node.width ?? (strict ? 0 : 200)
  const height = node.height ?? (strict ? 0 : 150)
  const zIndex = node.zIndex ?? 0
  if (
    !id ||
    !type ||
    (strict &&
      (!Number.isFinite(x) ||
        !Number.isFinite(y) ||
        !Number.isFinite(width) ||
        !Number.isFinite(height) ||
        !Number.isInteger(zIndex) ||
        width <= 0 ||
        height <= 0))
  )
    return null

  const currentGeometry = canvasGeometryFromProto(node.geometry)
  const geometry =
    currentGeometry ??
    (node.geometry ? null : decodeLegacyCanvasGeometry(node.shapeData))
  if (
    strict &&
    (type === 'drawing' || type === 'shape') &&
    (node.geometry || node.shapeData?.length) &&
    !geometry
  )
    return null

  return {
    id,
    x,
    y,
    width,
    height,
    zIndex,
    type,
    textContent: node.textContent || undefined,
    geometry: geometry ?? undefined,
    objectKey: node.objectKey || undefined,
    pinned: node.pinned,
    viewPath: node.viewPath || undefined,
  }
}

export function canvasNodeFromProto(
  node: ProtoCanvasNode,
): CanvasNodeData | null {
  return parseCanvasNode(node, undefined, true)
}

export function canvasNodeFromPersistedProto(
  node: ProtoCanvasNode,
  mapKey: string,
): CanvasNodeData {
  return parseCanvasNode(node, mapKey, false)!
}

export function canvasNodeToProto(node: CanvasNodeData): ProtoCanvasNode {
  return {
    id: node.id,
    x: node.x,
    y: node.y,
    width: node.width,
    height: node.height,
    zIndex: node.zIndex,
    type: nodeTypeToProto(node.type),
    textContent: node.textContent ?? '',
    // Field 9 remains a bounded compatibility projection until legacy readers retire.
    shapeData: encodeLegacyCanvasGeometry(node.geometry),
    geometry: canvasGeometryToProto(node.geometry),
    objectKey: node.objectKey ?? '',
    pinned: node.pinned ?? false,
    viewPath: node.viewPath ?? '',
  }
}

export function isCurrentCanvasGeometryKind(
  kind: CanvasGeometryKind | undefined,
): boolean {
  return (
    kind !== undefined &&
    kind >= CanvasGeometryKind.PEN &&
    kind <= CanvasGeometryKind.ELLIPSE
  )
}

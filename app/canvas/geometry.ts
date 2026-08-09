import {
  CanvasGeometryKind as ProtoCanvasGeometryKind,
  type CanvasGeometry as ProtoCanvasGeometry,
} from '@s4wave/sdk/canvas/canvas.pb.js'

export type CanvasGeometryKind =
  | 'pen'
  | 'line'
  | 'arrow'
  | 'rectangle'
  | 'ellipse'

export interface CanvasPoint {
  x: number
  y: number
}

export interface CanvasGeometry {
  kind: CanvasGeometryKind
  color: string
  points: CanvasPoint[]
}

export const DEFAULT_CANVAS_COLOR = '#2563eb'

const protoKinds: Record<CanvasGeometryKind, ProtoCanvasGeometryKind> = {
  pen: ProtoCanvasGeometryKind.PEN,
  line: ProtoCanvasGeometryKind.LINE,
  arrow: ProtoCanvasGeometryKind.ARROW,
  rectangle: ProtoCanvasGeometryKind.RECTANGLE,
  ellipse: ProtoCanvasGeometryKind.ELLIPSE,
}

function geometryKindFromProto(
  kind: ProtoCanvasGeometryKind | undefined,
): CanvasGeometryKind | null {
  switch (kind) {
    case ProtoCanvasGeometryKind.PEN:
      return 'pen'
    case ProtoCanvasGeometryKind.LINE:
      return 'line'
    case ProtoCanvasGeometryKind.ARROW:
      return 'arrow'
    case ProtoCanvasGeometryKind.RECTANGLE:
      return 'rectangle'
    case ProtoCanvasGeometryKind.ELLIPSE:
      return 'ellipse'
    default:
      return null
  }
}

export function canvasGeometryFromProto(
  geometry: ProtoCanvasGeometry | undefined,
): CanvasGeometry | null {
  if (!geometry) return null
  const kind = geometryKindFromProto(geometry.kind)
  const points = (geometry.points ?? []).map((point) => ({
    x: point.x ?? 0,
    y: point.y ?? 0,
  }))
  if (!kind || !geometry.color || points.length < 2) return null
  if (
    points.some(
      (point) => !Number.isFinite(point.x) || !Number.isFinite(point.y),
    )
  )
    return null
  return { kind, color: geometry.color, points }
}

// encodeLegacyCanvasGeometry produces the field-9 JSON read by supported legacy clients.
export function encodeLegacyCanvasGeometry(
  geometry: CanvasGeometry | undefined,
): Uint8Array | undefined {
  return geometry
    ? new TextEncoder().encode(JSON.stringify(geometry))
    : undefined
}

export function canvasGeometryToProto(
  geometry: CanvasGeometry | undefined,
): ProtoCanvasGeometry | undefined {
  if (!geometry) return undefined
  return {
    kind: protoKinds[geometry.kind],
    color: geometry.color,
    points: geometry.points.map(({ x, y }) => ({ x, y })),
  }
}

// decodeLegacyCanvasGeometry is confined to the persisted Canvas-node migration boundary.
export function decodeLegacyCanvasGeometry(
  data: Uint8Array | undefined,
): CanvasGeometry | null {
  if (!data?.length) return null
  try {
    const decoded: unknown = JSON.parse(new TextDecoder().decode(data))
    const candidate = Array.isArray(decoded)
      ? { kind: 'pen', color: 'currentColor', points: decoded }
      : decoded
    if (typeof candidate !== 'object' || candidate === null) return null
    const value = candidate as {
      kind?: unknown
      color?: unknown
      points?: unknown
    }
    if (
      typeof value.kind !== 'string' ||
      !(value.kind in protoKinds) ||
      typeof value.color !== 'string' ||
      !Array.isArray(value.points)
    )
      return null
    const points = value.points.flatMap((point) => {
      if (typeof point !== 'object' || point === null) return []
      const { x, y } = point as { x?: unknown; y?: unknown }
      return typeof x === 'number' &&
        Number.isFinite(x) &&
        typeof y === 'number' &&
        Number.isFinite(y)
        ? [{ x, y }]
        : []
    })
    return points.length >= 2
      ? { kind: value.kind as CanvasGeometryKind, color: value.color, points }
      : null
  } catch {
    return null
  }
}

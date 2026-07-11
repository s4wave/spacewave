// CanvasGeometryKind identifies geometry stored in a Canvas node's shapeData.
export type CanvasGeometryKind =
  | 'pen'
  | 'line'
  | 'arrow'
  | 'rectangle'
  | 'ellipse'

// CanvasPoint is a point relative to a Canvas node's origin.
export interface CanvasPoint {
  x: number
  y: number
}

// CanvasGeometry is the shared persisted model for drawings and shapes.
export interface CanvasGeometry {
  kind: CanvasGeometryKind
  color: string
  points: CanvasPoint[]
}

// DEFAULT_CANVAS_COLOR is the initial color for new drawings and shapes.
export const DEFAULT_CANVAS_COLOR = '#2563eb'

function parsePoint(value: unknown): CanvasPoint | null {
  if (typeof value !== 'object' || value === null) return null
  const point = value as { x?: unknown; y?: unknown }
  if (typeof point.x !== 'number' || typeof point.y !== 'number') return null
  return { x: point.x, y: point.y }
}

function isGeometryKind(value: unknown): value is CanvasGeometryKind {
  return (
    value === 'pen' ||
    value === 'line' ||
    value === 'arrow' ||
    value === 'rectangle' ||
    value === 'ellipse'
  )
}

// encodeCanvasGeometry encodes the shared drawing and shape payload.
export function encodeCanvasGeometry(geometry: CanvasGeometry): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(geometry))
}

// decodeCanvasGeometry decodes current payloads and legacy point arrays.
export function decodeCanvasGeometry(
  data: Uint8Array | undefined,
): CanvasGeometry | null {
  if (!data?.length) return null
  try {
    const decoded: unknown = JSON.parse(new TextDecoder().decode(data))
    if (Array.isArray(decoded)) {
      const points = decoded.flatMap((value) => {
        const point = parsePoint(value)
        return point ? [point] : []
      })
      return points.length >= 2
        ? { kind: 'pen', color: 'currentColor', points }
        : null
    }
    if (typeof decoded !== 'object' || decoded === null) return null
    const candidate = decoded as {
      kind?: unknown
      color?: unknown
      points?: unknown
    }
    if (
      !isGeometryKind(candidate.kind) ||
      typeof candidate.color !== 'string' ||
      !Array.isArray(candidate.points)
    ) {
      return null
    }
    const points = candidate.points.flatMap((value) => {
      const point = parsePoint(value)
      return point ? [point] : []
    })
    return points.length >= 2
      ? { kind: candidate.kind, color: candidate.color, points }
      : null
  } catch {
    return null
  }
}

import {
  CanvasClipboardPayload,
  CanvasClipboardVersion,
} from '@s4wave/sdk/canvas/canvas.pb.js'

import { canvasNodeFromProto, canvasNodeToProto } from './canvas-node-proto.js'
import type { CanvasNodeData } from './types.js'

const CANVAS_CLIPBOARD_PREFIX = 'spacewave-canvas:v1:'
const MAX_CLIPBOARD_BASE64_LENGTH = 4 * 1024 * 1024
const MAX_CLIPBOARD_NODES = 1000
const MAX_GEOMETRY_POINTS = 10000
const MAX_CANVAS_COORDINATE = 1_000_000_000
const MAX_CANVAS_DIMENSION = 1_000_000
const MAX_CANVAS_NODE_ID_BYTES = 1024
const MAX_CANVAS_TEXT_BYTES = 1024 * 1024
const MAX_CANVAS_PATH_BYTES = 4096
const PASTE_OFFSET = 20
const utf8Encoder = new TextEncoder()

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function base64ToBytes(encoded: string): Uint8Array | null {
  try {
    const binary = atob(encoded)
    const bytes = Uint8Array.from(binary, (character) =>
      character.charCodeAt(0),
    )
    return bytesToBase64(bytes) === encoded ? bytes : null
  } catch {
    return null
  }
}

function utf8Length(value: string | undefined): number {
  return utf8Encoder.encode(value ?? '').length
}

// isValidCanvasClipboardNode matches the durable CanvasNode bounds enforced by the Go mutation boundary.
export function isValidCanvasClipboardNode(node: CanvasNodeData): boolean {
  const geometry = node.geometry
  return (
    utf8Length(node.id) >= 1 &&
    utf8Length(node.id) <= MAX_CANVAS_NODE_ID_BYTES &&
    Number.isFinite(node.x) &&
    Math.abs(node.x) <= MAX_CANVAS_COORDINATE &&
    Number.isFinite(node.y) &&
    Math.abs(node.y) <= MAX_CANVAS_COORDINATE &&
    Number.isFinite(node.width) &&
    node.width > 0 &&
    node.width <= MAX_CANVAS_DIMENSION &&
    Number.isFinite(node.height) &&
    node.height > 0 &&
    node.height <= MAX_CANVAS_DIMENSION &&
    Number.isInteger(node.zIndex) &&
    utf8Length(node.textContent) <= MAX_CANVAS_TEXT_BYTES &&
    utf8Length(node.objectKey) <= MAX_CANVAS_PATH_BYTES &&
    utf8Length(node.viewPath) <= MAX_CANVAS_PATH_BYTES &&
    (!geometry ||
      ((node.type === 'shape' || node.type === 'drawing') &&
        (geometry.color === 'currentColor' ||
          /^#[0-9a-fA-F]{6}$/.test(geometry.color)) &&
        geometry.points.length >= 2 &&
        geometry.points.length <= MAX_GEOMETRY_POINTS &&
        geometry.points.every(
          (point) =>
            Number.isFinite(point.x) &&
            Math.abs(point.x) <= MAX_CANVAS_COORDINATE &&
            Number.isFinite(point.y) &&
            Math.abs(point.y) <= MAX_CANVAS_COORDINATE,
        )))
  )
}

export function encodeCanvasClipboard(nodes: CanvasNodeData[]): string {
  const bytes = CanvasClipboardPayload.toBinary({
    version: CanvasClipboardVersion.V1,
    nodes: nodes.map(canvasNodeToProto),
  })
  return CANVAS_CLIPBOARD_PREFIX + bytesToBase64(bytes)
}

// decodeCanvasClipboardPaste returns only final translated nodes that the mutation boundary will accept.
export function decodeCanvasClipboardPaste(
  text: string,
  copiedAt: number,
): Map<string, CanvasNodeData> | null {
  if (!text.startsWith(CANVAS_CLIPBOARD_PREFIX)) return null
  const encoded = text.slice(CANVAS_CLIPBOARD_PREFIX.length)
  if (!encoded || encoded.length > MAX_CLIPBOARD_BASE64_LENGTH) return null
  const bytes = base64ToBytes(encoded)
  if (!bytes) return null
  try {
    const payload = CanvasClipboardPayload.fromBinary(bytes)
    if (
      payload.version !== CanvasClipboardVersion.V1 ||
      !payload.nodes?.length ||
      payload.nodes.length > MAX_CLIPBOARD_NODES
    )
      return null

    const nodes = new Map<string, CanvasNodeData>()
    for (const [index, protoNode] of payload.nodes.entries()) {
      const source = canvasNodeFromProto(protoNode)
      if (!source) return null
      const node = {
        ...source,
        id: `${source.id}-copy-${copiedAt}-${index}`,
        x: source.x + PASTE_OFFSET,
        y: source.y + PASTE_OFFSET,
      }
      if (!isValidCanvasClipboardNode(node) || nodes.has(node.id)) return null
      nodes.set(node.id, node)
    }
    return nodes
  } catch {
    return null
  }
}

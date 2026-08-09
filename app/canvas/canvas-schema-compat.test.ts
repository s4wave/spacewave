import { createEnumType } from '@aptre/protobuf-es-lite/enum'
import { createMessageType } from '@aptre/protobuf-es-lite/message'
import { ScalarType } from '@aptre/protobuf-es-lite/scalar'
import type { PartialFieldInfo } from '@aptre/protobuf-es-lite/field'
import { describe, expect, it } from 'vitest'

import {
  CanvasEdge,
  CanvasNode,
  EdgeStyle,
  NodeType,
} from '@s4wave/sdk/canvas/canvas.pb.js'

import { canvasNodeFromProto, canvasNodeToProto } from './canvas-node-proto.js'

interface LegacyCanvasNode {
  id?: string
  x?: number
  y?: number
  width?: number
  height?: number
  zIndex?: number
  type?: number
  textContent?: string
  shapeData?: Uint8Array
  objectKey?: string
  pinned?: boolean
  viewPath?: string
}

const legacyNodeType = createEnumType('s4wave.canvas.NodeType', [
  [0, 'NODE_TYPE_UNKNOWN'],
  [1, 'NODE_TYPE_TEXT'],
  [2, 'NODE_TYPE_SHAPE'],
  [3, 'NODE_TYPE_WORLD_OBJECT'],
  [4, 'NODE_TYPE_DRAWING'],
])

// legacyCanvasNodeDescriptor is the released field-1-through-12 reader and writer.
const legacyCanvasNodeDescriptor = createMessageType<LegacyCanvasNode>({
  typeName: 's4wave.canvas.CanvasNode',
  fields: [
    { no: 1, name: 'id', kind: 'scalar', T: ScalarType.STRING },
    { no: 2, name: 'x', kind: 'scalar', T: ScalarType.DOUBLE },
    { no: 3, name: 'y', kind: 'scalar', T: ScalarType.DOUBLE },
    { no: 4, name: 'width', kind: 'scalar', T: ScalarType.DOUBLE },
    { no: 5, name: 'height', kind: 'scalar', T: ScalarType.DOUBLE },
    { no: 6, name: 'z_index', kind: 'scalar', T: ScalarType.INT32 },
    { no: 7, name: 'type', kind: 'enum', T: legacyNodeType },
    { no: 8, name: 'text_content', kind: 'scalar', T: ScalarType.STRING },
    { no: 9, name: 'shape_data', kind: 'scalar', T: ScalarType.BYTES },
    { no: 10, name: 'object_key', kind: 'scalar', T: ScalarType.STRING },
    { no: 11, name: 'pinned', kind: 'scalar', T: ScalarType.BOOL },
    { no: 12, name: 'view_path', kind: 'scalar', T: ScalarType.STRING },
  ] satisfies readonly PartialFieldInfo[],
  packedByDefault: true,
})

interface LegacyCanvasEdge {
  id?: string
  sourceNodeId?: string
  targetNodeId?: string
  label?: string
  style?: number
}

const legacyEdgeStyle = createEnumType('s4wave.canvas.EdgeStyle', [
  [0, 'EDGE_STYLE_BEZIER'],
  [1, 'EDGE_STYLE_STRAIGHT'],
])

// legacyEdgeStyleToCanvas reproduces the released wire-zero bezier renderer.
function legacyEdgeStyleToCanvas(
  style: number | undefined,
): 'bezier' | 'straight' {
  return style === 1 ? 'straight' : 'bezier'
}

const legacyCanvasEdgeDescriptor = createMessageType<LegacyCanvasEdge>({
  typeName: 's4wave.canvas.CanvasEdge',
  fields: [
    { no: 1, name: 'id', kind: 'scalar', T: ScalarType.STRING },
    { no: 2, name: 'source_node_id', kind: 'scalar', T: ScalarType.STRING },
    { no: 3, name: 'target_node_id', kind: 'scalar', T: ScalarType.STRING },
    { no: 4, name: 'label', kind: 'scalar', T: ScalarType.STRING },
    { no: 5, name: 'style', kind: 'enum', T: legacyEdgeStyle },
  ] satisfies readonly PartialFieldInfo[],
  packedByDefault: true,
})

describe('Canvas schema compatibility', () => {
  it('falls back to field 9 after a released old writer drops field 13', () => {
    const next = canvasNodeToProto({
      id: 'shape',
      x: 1,
      y: 2,
      width: 100,
      height: 80,
      zIndex: 3,
      type: 'shape',
      geometry: {
        kind: 'rectangle',
        color: '#123456',
        points: [
          { x: 0, y: 0 },
          { x: 100, y: 80 },
        ],
      },
    })
    const oldRead = legacyCanvasNodeDescriptor.fromBinary(
      CanvasNode.toBinary(next),
    )
    expect(new TextDecoder().decode(oldRead.shapeData)).toContain('rectangle')

    const oldMutation: LegacyCanvasNode = {
      id: oldRead.id,
      x: 25,
      y: oldRead.y,
      width: oldRead.width,
      height: oldRead.height,
      zIndex: oldRead.zIndex,
      type: oldRead.type,
      textContent: oldRead.textContent,
      shapeData: oldRead.shapeData,
      objectKey: oldRead.objectKey,
      pinned: oldRead.pinned,
      viewPath: oldRead.viewPath,
    }
    const oldWire = legacyCanvasNodeDescriptor.toBinary(oldMutation)
    const newRead = CanvasNode.fromBinary(oldWire)
    expect(newRead.x).toBe(25)
    expect(newRead.geometry).toBeUndefined()
    expect(canvasNodeFromProto(newRead)?.geometry).toEqual({
      kind: 'rectangle',
      color: '#123456',
      points: [
        { x: 0, y: 0 },
        { x: 100, y: 80 },
      ],
    })
  })

  it('reads legacy field 9 with the new descriptor', () => {
    const oldWire = legacyCanvasNodeDescriptor.toBinary({
      id: 'pen',
      x: 0,
      y: 0,
      width: 10,
      height: 10,
      type: NodeType.DRAWING,
      shapeData: new TextEncoder().encode(
        '{"kind":"pen","color":"#abcdef","points":[{"x":0,"y":0},{"x":1,"y":1}]}',
      ),
    })
    expect(
      canvasNodeFromProto(CanvasNode.fromBinary(oldWire))?.geometry?.kind,
    ).toBe('pen')
  })

  it('keeps released and current bezier on wire zero for released readers', () => {
    const releasedWire = legacyCanvasEdgeDescriptor.toBinary({
      id: 'released',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: 0,
    })
    expect(CanvasEdge.fromBinary(releasedWire).style ?? EdgeStyle.BEZIER).toBe(
      EdgeStyle.BEZIER,
    )

    const currentWire = CanvasEdge.toBinary({
      id: 'current',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: EdgeStyle.BEZIER,
    })
    const releasedRead = legacyCanvasEdgeDescriptor.fromBinary(currentWire)
    expect(releasedRead.style ?? 0).toBe(0)
    expect(legacyEdgeStyleToCanvas(releasedRead.style)).toBe('bezier')
  })

  it('keeps straight on released wire one', () => {
    const currentWire = CanvasEdge.toBinary({
      id: 'straight',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: EdgeStyle.STRAIGHT,
    })
    const releasedRead = legacyCanvasEdgeDescriptor.fromBinary(currentWire)
    expect(releasedRead.style).toBe(1)
    expect(legacyEdgeStyleToCanvas(releasedRead.style)).toBe('straight')
  })
})

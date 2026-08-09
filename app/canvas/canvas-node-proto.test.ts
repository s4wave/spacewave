import { describe, expect, it } from 'vitest'

import {
  CanvasGeometryKind,
  CanvasNode,
  NodeType,
} from '@s4wave/sdk/canvas/canvas.pb.js'

import {
  canvasNodeFromPersistedProto,
  canvasNodeFromProto,
  canvasNodeToProto,
} from './canvas-node-proto.js'

const baseNode = {
  id: 'drawing',
  x: 1,
  y: 2,
  width: 100,
  height: 80,
  zIndex: 3,
  type: NodeType.DRAWING,
}

describe('Canvas node geometry persistence boundary', () => {
  it('reads legacy JSON geometry and dual-writes legacy and current fields', () => {
    const node = canvasNodeFromProto({
      ...baseNode,
      shapeData: new TextEncoder().encode(
        JSON.stringify([
          { x: 1, y: 2 },
          { x: 3, y: 4 },
        ]),
      ),
    })

    expect(node?.geometry).toEqual({
      kind: 'pen',
      color: 'currentColor',
      points: [
        { x: 1, y: 2 },
        { x: 3, y: 4 },
      ],
    })
    const current = canvasNodeToProto(node!)
    expect(new TextDecoder().decode(current.shapeData)).toContain(
      '"kind":"pen"',
    )
    expect(current.geometry?.kind).toBe(CanvasGeometryKind.PEN)
  })

  it('prefers current generated geometry over the legacy field', () => {
    const node = canvasNodeFromProto({
      ...baseNode,
      geometry: {
        kind: CanvasGeometryKind.LINE,
        color: '#123456',
        points: [
          { x: 5, y: 6 },
          { x: 7, y: 8 },
        ],
      },
      shapeData: new TextEncoder().encode('[{"x":0,"y":0},{"x":1,"y":1}]'),
    })
    expect(node?.geometry?.kind).toBe('line')
    expect(node?.geometry?.color).toBe('#123456')
  })

  it('round-trips current geometry through the generated Canvas node wire codec', () => {
    const proto = {
      ...baseNode,
      geometry: {
        kind: CanvasGeometryKind.RECTANGLE,
        color: '#654321',
        points: [
          { x: 1.5, y: 2.5 },
          { x: 30.5, y: 40.5 },
        ],
      },
    }
    const decoded = CanvasNode.fromBinary(CanvasNode.toBinary(proto))
    expect(canvasNodeFromProto(decoded)?.geometry).toEqual({
      kind: 'rectangle',
      color: '#654321',
      points: [
        { x: 1.5, y: 2.5 },
        { x: 30.5, y: 40.5 },
      ],
    })
  })

  it('preserves legacy persisted defaults without weakening clipboard validation', () => {
    expect(
      canvasNodeFromPersistedProto({ id: 'legacy' }, 'legacy'),
    ).toMatchObject({
      id: 'legacy',
      type: 'text',
      width: 200,
      height: 150,
    })
    expect(canvasNodeFromProto({ id: 'untrusted' })).toBeNull()
  })
})

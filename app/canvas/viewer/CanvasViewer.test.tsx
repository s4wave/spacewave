import { describe, expect, it } from 'vitest'

import { EdgeStyle } from '@s4wave/sdk/canvas/canvas.pb.js'

import { protoEdgeStyleToCanvas } from './CanvasViewer.js'
import { isCanvasInsertableObject } from './object-picker.js'

describe('isCanvasInsertableObject', () => {
  it('hides the reserved space settings object', () => {
    expect(
      isCanvasInsertableObject('settings', 'space/settings', 'canvas-1'),
    ).toBe(false)
  })

  it('keeps regular objects available for insertion', () => {
    expect(
      isCanvasInsertableObject(
        'object-layout/main',
        'alpha/object-layout',
        'canvas-1',
      ),
    ).toBe(true)
  })
})

describe('protoEdgeStyleToCanvas', () => {
  it('renders released and current wire-zero bezier as bezier', () => {
    expect(protoEdgeStyleToCanvas(0 as EdgeStyle)).toBe('bezier')
    expect(protoEdgeStyleToCanvas(EdgeStyle.BEZIER)).toBe('bezier')
  })

  it('renders wire-one straight as straight', () => {
    expect(protoEdgeStyleToCanvas(EdgeStyle.STRAIGHT)).toBe('straight')
  })
})

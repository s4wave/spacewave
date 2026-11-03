import { expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { userEvent } from 'vitest/browser'
import '@s4wave/web/style/app.js'

import { CanvasMinimap } from './CanvasMinimap.js'
import { CanvasNode } from './CanvasNode.js'
import { CanvasDrawingLayer } from './CanvasDrawingLayer.js'
import { CanvasEdgeLayer } from './CanvasEdgeLayer.js'
import { CanvasSelectionOverlay } from './CanvasSelectionOverlay.js'
import type { CanvasNodeData, Viewport, EphemeralEdge } from './types.js'

it('projects nodes and centers the viewport through pointer and keyboard input', async () => {
  // Render real geometry through the native stylesheet and its accessible control.
  const nodes = new Map<string, CanvasNodeData>([
    [
      'a',
      { id: 'a', x: 0, y: 0, width: 200, height: 100, zIndex: 0, type: 'text' },
    ],
    [
      'b',
      {
        id: 'b',
        x: 400,
        y: 300,
        width: 200,
        height: 100,
        zIndex: 1,
        type: 'text',
      },
    ],
  ])
  const viewport: Viewport = { x: 0, y: 0, scale: 2 }
  const onViewportChange = vi.fn<(viewport: Viewport) => void>()
  const view = await render(
    <CanvasMinimap
      nodes={nodes}
      viewport={viewport}
      containerSize={{ width: 800, height: 600 }}
      onViewportChange={onViewportChange}
    />,
  )
  const minimap = view.getByRole('button', { name: 'Center canvas minimap' })
  const element = minimap.element() as HTMLElement

  // Both node rectangles and the viewport overlay occupy actual, bounded CSS geometry.
  expect(element.getBoundingClientRect().width).toBe(200)
  expect(element.getBoundingClientRect().height).toBe(150)
  expect(element.children).toHaveLength(3)
  const first = element.children[0].getBoundingClientRect()
  const second = element.children[1].getBoundingClientRect()
  expect(first.width).toBeGreaterThan(0)
  expect(first.width).toBeLessThanOrEqual(200)
  expect(second.left).toBeGreaterThan(first.right)

  // Pointer and keyboard activation center the same point without changing zoom.
  await minimap.click()
  const pointer = onViewportChange.mock.calls.at(-1)?.[0]
  expect(pointer).toBeDefined()
  expect(pointer?.scale).toBe(2)
  expect(Number.isFinite(pointer?.x)).toBe(true)
  expect(Number.isFinite(pointer?.y)).toBe(true)

  // Keyboard activation targets the same center as the pointer.
  element.focus()
  await userEvent.keyboard('{Enter}')
  expect(onViewportChange.mock.calls.at(-1)?.[0]).toEqual(pointer)

  // Emptying the scene retains its viewport overlay and keyboard navigation.
  await view.rerender(
    <CanvasMinimap
      nodes={new Map()}
      viewport={viewport}
      containerSize={{ width: 800, height: 600 }}
      onViewportChange={onViewportChange}
    />,
  )
  expect(element.children).toHaveLength(1)
  onViewportChange.mockClear()
  element.focus()
  await userEvent.keyboard(' ')
  expect(onViewportChange).toHaveBeenCalledOnce()
  expect(onViewportChange.mock.calls[0]?.[0].scale).toBe(2)
})

it('retains canvas geometry, zoom presentation and drawing hit targets', async () => {
  // Render the production node with dynamic stacking and inline geometry together.
  const node: CanvasNodeData = {
    id: 'native-canvas-node',
    x: 100,
    y: 200,
    width: 300,
    height: 250,
    zIndex: 7,
    type: 'text',
    textContent: '  line one\n  line two',
  }
  const props = {
    node,
    visible: true,
    focus: 'border' as const,
    callbacks: {
      onNodesChange: vi.fn(),
      onEdgesChange: vi.fn(),
      onNodeSelect: vi.fn(),
    },
    onSelectWithFocus: vi.fn(),
    onMove: vi.fn(),
    onMoveEnd: vi.fn(),
  }
  const view = await render(<CanvasNode {...props} scale={1} selected />)
  const element = document.querySelector<HTMLElement>(
    '[data-canvas-node="native-canvas-node"]',
  )!

  // Inline lengths and dynamic stacking survive the same intrinsic element.
  const css = getComputedStyle(element)
  expect([css.left, css.top, css.width, css.height, css.zIndex]).toEqual([
    '100px',
    '200px',
    '300px',
    '250px',
    '7',
  ])
  expect(css.touchAction).toBe('none')

  // Detailed content retains whitespace and all four resize targets.
  expect(element.querySelector('pre')?.textContent).toBe(node.textContent)
  expect(getComputedStyle(element.querySelector('pre')!).whiteSpace).toBe(
    'pre-wrap',
  )
  const resizeHandles = () =>
    Array.from(element.querySelectorAll('div')).filter((handle) =>
      getComputedStyle(handle).cursor.endsWith('-resize'),
    )
  expect(resizeHandles()).toHaveLength(4)

  // Zooming out removes the detailed content and selection handles.
  await view.rerender(<CanvasNode {...props} scale={0.2} selected={false} />)
  expect(element.querySelector('pre')).toBeNull()
  expect(element.textContent).toContain('text')
  expect(resizeHandles()).toHaveLength(0)

  // Drawing owns pointer input only while its tool is active.
  await view.rerender(
    <CanvasDrawingLayer visible viewport={{ x: 0, y: 0, scale: 1 }} />,
  )
  const canvas = document.querySelector('canvas')!
  expect([
    getComputedStyle(canvas).pointerEvents,
    getComputedStyle(canvas).cursor,
    getComputedStyle(canvas).zIndex,
  ]).toEqual(['auto', 'crosshair', '10'])
  await view.rerender(
    <CanvasDrawingLayer visible={false} viewport={{ x: 0, y: 0, scale: 1 }} />,
  )
  expect([
    getComputedStyle(canvas).pointerEvents,
    getComputedStyle(canvas).zIndex,
  ]).toEqual(['none', '-1'])

  // Reverse dragging produces the same selection bounds as forward dragging.
  const container = canvas.parentElement!
  await view.rerender(
    <CanvasSelectionOverlay
      dragRect={{ startX: 200, startY: 180, endX: 50, endY: 30 }}
    />,
  )
  const overlay = container.firstElementChild!
  const selection = getComputedStyle(overlay)
  expect([
    selection.left,
    selection.top,
    selection.width,
    selection.height,
    selection.pointerEvents,
  ]).toEqual(['50px', '30px', '150px', '150px', 'none'])

  // Graph-link pills accept pointer input through the otherwise passive SVG layer.
  const edge: EphemeralEdge = {
    renderKey: 'eq1',
    subject: '<object-a>',
    predicate: 'relatedTo',
    object: '<object-b>',
    sourceNodeId: 'a',
    sourceObjectKey: 'objects/a',
    sourceGroupKey: 'a',
    sourceGroupIndex: 0,
    sourceGroupOffset: 0,
    outgoingTruncated: false,
    incomingTruncated: false,
    hiddenCount: 0,
    direction: 'out',
    linkedObjectKey: 'obj-key',
    linkedObjectLabel: 'Linked object',
    hideable: true,
    userRemovable: true,
    protected: false,
    ownerManaged: false,
    stubX: 400,
    stubY: 200,
  }
  const onPinObject = vi.fn()
  await view.rerender(
    <div style={{ position: 'relative', width: 800, height: 600 }}>
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[edge]}
        nodes={
          new Map([
            ['a', { ...node, id: 'a', x: 0, y: 0, width: 100, height: 100 }],
          ])
        }
        callbacks={{ onPinObject }}
      />
    </div>,
  )
  expect(
    getComputedStyle(document.querySelector('foreignObject')!).pointerEvents,
  ).toBe('auto')
  await view.getByTitle('Load Linked object', { exact: true }).click()
  expect(onPinObject).toHaveBeenCalledOnce()
  expect(onPinObject.mock.calls[0]?.[0]).toBe('obj-key')
})

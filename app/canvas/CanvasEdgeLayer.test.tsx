import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CanvasEdgeLayer } from './CanvasEdgeLayer.js'
import type {
  CanvasNodeData,
  CanvasEdgeData,
  EphemeralEdge,
  CanvasCallbacks,
} from './types.js'

function makeNode(overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
  return {
    id: 'n1',
    x: 0,
    y: 0,
    width: 100,
    height: 100,
    zIndex: 0,
    type: 'text',
    ...overrides,
  }
}

function makeCallbacks(
  overrides: Partial<CanvasCallbacks> = {},
): CanvasCallbacks {
  return {
    onPinObject: vi.fn(),
    ...overrides,
  }
}

function makeEphemeralEdge(
  overrides: Partial<EphemeralEdge> = {},
): EphemeralEdge {
  return {
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
    linkedObjectLabel: 'obj-key',
    hideable: true,
    userRemovable: true,
    protected: false,
    ownerManaged: false,
    ...overrides,
  }
}

describe('CanvasEdgeLayer', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders an SVG element', () => {
    const nodes = new Map<string, CanvasNodeData>()
    render(
      <CanvasEdgeLayer edges={[]} nodes={nodes} callbacks={makeCallbacks()} />,
    )
    const svg = document.querySelector('svg')
    expect(svg).toBeTruthy()
  })

  it('renders arrowhead marker definitions', () => {
    const nodes = new Map<string, CanvasNodeData>()
    render(
      <CanvasEdgeLayer edges={[]} nodes={nodes} callbacks={makeCallbacks()} />,
    )
    expect(document.getElementById('canvas-arrowhead')).toBeTruthy()
    expect(document.getElementById('canvas-arrowhead-dashed')).toBeTruthy()
  })

  it('renders a bezier path for a bezier edge', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const n2 = makeNode({ id: 'b', x: 300, y: 200, width: 100, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const edge: CanvasEdgeData = {
      id: 'e1',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: 'bezier',
    }
    render(
      <CanvasEdgeLayer
        edges={[edge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )
    const paths = document.querySelectorAll('path')
    expect(paths.length).toBeGreaterThan(0)
    // Bezier paths contain C (cubic bezier command).
    const d = paths[0].getAttribute('d') ?? ''
    expect(d).toContain('C')
  })

  it('renders a straight line path for a straight edge', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const n2 = makeNode({ id: 'b', x: 300, y: 0, width: 100, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const edge: CanvasEdgeData = {
      id: 'e1',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: 'straight',
    }
    render(
      <CanvasEdgeLayer
        edges={[edge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )
    const paths = document.querySelectorAll('path')
    expect(paths.length).toBeGreaterThan(0)
    // Straight paths contain L (line-to command) and no C.
    const d = paths[0].getAttribute('d') ?? ''
    expect(d).toContain('L')
    expect(d).not.toContain('C')
  })

  it('skips edges when source or target node is missing', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0 })
    const nodes = new Map([['a', n1]])
    const edge: CanvasEdgeData = {
      id: 'e1',
      sourceNodeId: 'a',
      targetNodeId: 'missing',
      style: 'bezier',
    }
    render(
      <CanvasEdgeLayer
        edges={[edge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )
    // No edge paths should be rendered (only the defs markers exist).
    const paths = document.querySelectorAll('path')
    expect(paths.length).toBe(0)
  })

  it('renders dashed stroke for ephemeral edges', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const n2 = makeNode({ id: 'b', x: 300, y: 0, width: 100, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const ephEdge = makeEphemeralEdge({
      targetNodeId: 'b',
    })
    render(
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[ephEdge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )
    const dashedPaths = document.querySelectorAll('path[stroke-dasharray]')
    expect(dashedPaths.length).toBeGreaterThan(0)
  })

  it('uses the explicit source node for ephemeral target edges', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const n2 = makeNode({ id: 'b', x: 300, y: 0, width: 100, height: 100 })
    const n3 = makeNode({ id: 'c', x: 600, y: 0, width: 100, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
      ['c', n3],
    ])
    const ephEdge = makeEphemeralEdge({
      sourceNodeId: 'c',
      targetNodeId: 'b',
    })
    render(
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[ephEdge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )
    const path = document.querySelector('path[stroke-dasharray]')
    expect(path?.getAttribute('d')).toMatch(/^M 650 50 /)
  })

  it('shows load pill for stub ephemeral edges', async () => {
    const user = userEvent.setup()
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const nodes = new Map([['a', n1]])
    const onPinObject = vi.fn()
    const callbacks = makeCallbacks({ onPinObject })
    const ephEdge = makeEphemeralEdge({
      stubX: 400,
      stubY: 200,
      linkedObjectLabel: 'Git Repo',
      linkedObjectTypeLabel: 'Git Repository',
    })
    render(
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[ephEdge]}
        nodes={nodes}
        callbacks={callbacks}
      />,
    )
    const pinBtn = document.querySelector('button[title*="Load Git Repo"]')
    expect(pinBtn).toBeTruthy()
    expect(pinBtn?.textContent).toContain('relatedTo')
    expect(pinBtn?.textContent).toContain('Git Repo')
    expect(pinBtn?.textContent).toContain('Git Repository')
    if (!pinBtn) throw new Error('load pill missing')
    await user.click(pinBtn)
    expect(onPinObject).toHaveBeenCalledWith('obj-key', 400, 200)
  })

  it('shows focus pill for loaded ephemeral edges', async () => {
    const user = userEvent.setup()
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const n2 = makeNode({ id: 'b', x: 300, y: 0, width: 100, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const onFocusObject = vi.fn()
    const onHideGraphLink = vi.fn()
    const onDeleteGraphLink = vi.fn()
    const callbacks = makeCallbacks({
      onFocusObject,
      onHideGraphLink,
      onDeleteGraphLink,
    })
    const ephEdge = makeEphemeralEdge({
      targetNodeId: 'b',
      linkedObjectLabel: 'Git Repo',
    })
    render(
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[ephEdge]}
        nodes={nodes}
        callbacks={callbacks}
      />,
    )

    const focusBtn = document.querySelector('button[title*="Focus Git Repo"]')
    expect(focusBtn).toBeTruthy()
    expect(focusBtn?.textContent).toContain('Focus')
    if (!focusBtn) throw new Error('focus pill missing')
    await user.click(focusBtn)
    expect(onFocusObject).toHaveBeenCalledWith('obj-key', 'b')

    const hideBtn = document.querySelector('button[title*="Hide relatedTo"]')
    expect(hideBtn).toBeTruthy()
    if (!hideBtn) throw new Error('hide button missing')
    await user.click(hideBtn)
    expect(onHideGraphLink).toHaveBeenCalledWith(ephEdge)

    const deleteBtn = document.querySelector(
      'button[title*="Delete relatedTo"]',
    )
    expect(deleteBtn).toBeTruthy()
    if (!deleteBtn) throw new Error('delete button missing')
    await user.click(deleteBtn)
    expect(onDeleteGraphLink).toHaveBeenCalledWith(ephEdge)
  })

  it('does not show delete for owner-managed graph links', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const nodes = new Map([['a', n1]])
    const ephEdge = makeEphemeralEdge({
      stubX: 400,
      stubY: 200,
      userRemovable: false,
      ownerManaged: true,
    })
    render(
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[ephEdge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )

    expect(document.querySelector('button[title*="Delete"]')).toBeNull()
  })

  it('shows compact graph-link summary badges', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const nodes = new Map([['a', n1]])
    const ephEdge = makeEphemeralEdge({
      stubX: 400,
      stubY: 200,
      outgoingTruncated: true,
      hiddenCount: 2,
    })
    render(
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[ephEdge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )

    const pill = document.querySelector('button[title*="Load"]')
    expect(pill?.textContent).toContain('capped')
    expect(pill?.textContent).toContain('hidden 2')
  })

  it('isolates graph-link pill pointer events from canvas drag gestures', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const nodes = new Map([['a', n1]])
    const ephEdge = makeEphemeralEdge({
      stubX: 400,
      stubY: 200,
    })
    render(
      <CanvasEdgeLayer
        edges={[]}
        ephemeralEdges={[ephEdge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )

    const pillHost = document.querySelector('foreignObject')
    expect(pillHost?.getAttribute('class')).toContain('pointer-events-auto')
  })

  it('renders edge labels on valid manual edge path identifiers', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const n2 = makeNode({ id: 'b', x: 300, y: 0, width: 100, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const edge: CanvasEdgeData = {
      id: 'edge with/slashes',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      label: 'depends on',
      style: 'bezier',
    }
    render(
      <CanvasEdgeLayer
        edges={[edge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )
    const textEl = document.querySelector('text')
    expect(textEl).toBeTruthy()
    expect(textEl?.textContent).toBe('depends on')

    const textPath = document.querySelector('textPath')
    const href = textPath?.getAttribute('href')
    expect(href).toMatch(/^#canvas-manual-edge-0-[A-Za-z0-9_-]+$/)
    expect(document.querySelector(`path[id="${href?.slice(1)}"]`)).toBeTruthy()
  })

  it('does not render label text when edge has no label', () => {
    const n1 = makeNode({ id: 'a', x: 0, y: 0, width: 100, height: 100 })
    const n2 = makeNode({ id: 'b', x: 300, y: 0, width: 100, height: 100 })
    const nodes = new Map([
      ['a', n1],
      ['b', n2],
    ])
    const edge: CanvasEdgeData = {
      id: 'e1',
      sourceNodeId: 'a',
      targetNodeId: 'b',
      style: 'bezier',
    }
    render(
      <CanvasEdgeLayer
        edges={[edge]}
        nodes={nodes}
        callbacks={makeCallbacks()}
      />,
    )
    const textEl = document.querySelector('text')
    expect(textEl).toBeFalsy()
  })
})

import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

interface CapturedObjectViewerProps {
  bottomBarId?: string
  exportUrl?: string
  objectInfo?: {
    info?: {
      case?: string
      value?: {
        objectKey?: string
      }
    }
  }
  onNavigate?: (to: { path: string }) => void
  path?: string
  standalone?: boolean
  stateNamespace?: string[]
}

const h = vi.hoisted(() => ({
  objectViewer: vi.fn((_props: CapturedObjectViewerProps) => null),
  onViewPathChange: vi.fn(),
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', () => ({
  ObjectViewer: (props: CapturedObjectViewerProps) => {
    h.objectViewer(props)
    return <div data-testid="object-viewer" />
  },
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContextSafe: () => ({
      spaceId: 'space/canvas',
    }),
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 7,
}))

import { CanvasObjectNode } from './CanvasObjectNode.js'

describe('CanvasObjectNode', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('scopes embedded ObjectViewer state and navigation to the canvas node', () => {
    render(
      <CanvasObjectNode
        objectKey="drive"
        canvasObjectKey="canvas/main"
        nodeId="node-1"
        worldState={{ value: {} } as never}
        viewPath="/docs"
        onViewPathChange={h.onViewPathChange}
      />,
    )

    const props = h.objectViewer.mock.calls[0]?.[0]
    expect(props?.standalone).toBe(true)
    expect(props?.bottomBarId).toBe('canvas-node-node-1')
    expect(props?.stateNamespace).toEqual([
      'canvas',
      'canvas/main',
      'node',
      'node-1',
      'viewer',
    ])
    expect(props?.objectInfo?.info?.case).toBe('worldObjectInfo')
    expect(props?.objectInfo?.info?.value?.objectKey).toBe('drive')
    expect(props?.path).toBe('/docs')
    props?.onNavigate?.({ path: '../readme.md' })
    expect(h.onViewPathChange).toHaveBeenCalledWith('/readme.md')
  })
})

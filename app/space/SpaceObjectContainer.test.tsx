import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { SpaceObjectContainer } from './SpaceObjectContainer.js'

interface CapturedObjectViewerProps {
  exportUrl?: string
  objectInfo?: {
    info?: {
      case?: string
      value?: {
        objectKey?: string
      }
    }
  }
  path?: string
}

const h = vi.hoisted(() => ({
  objectViewer: vi.fn((_props: CapturedObjectViewerProps) => null),
  navigateToRoot: vi.fn(),
  navigateToSubPath: vi.fn(),
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', () => ({
  ObjectViewer: (props: CapturedObjectViewerProps) => {
    h.objectViewer(props)
    return <div data-testid="object-viewer" />
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 7,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: () => ({
      spaceId: 'space/git',
      objectKey: 'repo/demo',
      objectPath: '',
      spaceWorldResource: { value: null, loading: false, error: null },
      navigateToRoot: h.navigateToRoot,
      navigateToSubPath: h.navigateToSubPath,
    }),
  },
}))

describe('SpaceObjectContainer', () => {
  it('passes the shared export endpoint to world object viewers', () => {
    render(<SpaceObjectContainer />)

    expect(h.objectViewer).toHaveBeenCalledTimes(1)
    const props = h.objectViewer.mock.calls[0]?.[0]
    expect(props?.exportUrl).toBe('/p/spacewave-core/export/u/7/so/space%2Fgit')
    expect(props?.objectInfo?.info?.case).toBe('worldObjectInfo')
    expect(props?.objectInfo?.info?.value?.objectKey).toBe('repo/demo')
    expect(props?.path).toBe('/')
  })
})

import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { SpaceObjectContainer } from './SpaceObjectContainer.js'

interface CapturedObjectViewerProps {
  exportUrl?: string
  objectInfo?: {
    info?: {
      case?: string
      value?: {
        objectKey?: string
        objectType?: string
      }
    }
  }
  path?: string
  onNavigate?: (to: { path: string }) => void
  stateNamespace?: string[]
}

const h = vi.hoisted(() => ({
  objectViewer: vi.fn((_props: CapturedObjectViewerProps) => null),
  navigateToRoot: vi.fn(),
  navigateToSubPath: vi.fn(),
  getQuickstartInitialObjectHandoff: vi.fn(),
  spaceContext: {
    spaceId: 'space/git',
    objectKey: 'repo/demo',
    objectPath: '',
    spaceState: {
      worldContents: {
        objects: [] as { objectKey?: string; objectType?: string }[],
      },
    },
    spaceWorldResource: { value: null, loading: false, error: null },
  },
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
      ...h.spaceContext,
      navigateToRoot: h.navigateToRoot,
      navigateToSubPath: h.navigateToSubPath,
    }),
  },
}))

vi.mock('@s4wave/app/quickstart/session-handoff.js', () => ({
  getQuickstartInitialObjectHandoff: h.getQuickstartInitialObjectHandoff,
}))

describe('SpaceObjectContainer', () => {
  beforeEach(() => {
    h.objectViewer.mockClear()
    h.navigateToRoot.mockClear()
    h.navigateToSubPath.mockClear()
    h.getQuickstartInitialObjectHandoff.mockReset()
    h.spaceContext = {
      spaceId: 'space/git',
      objectKey: 'repo/demo',
      objectPath: '',
      spaceState: { worldContents: { objects: [] } },
      spaceWorldResource: { value: null, loading: false, error: null },
    }
  })

  it('passes the shared export endpoint to world object viewers', () => {
    render(<SpaceObjectContainer />)

    expect(h.objectViewer).toHaveBeenCalledTimes(1)
    const props = h.objectViewer.mock.calls[0]?.[0]
    expect(props?.exportUrl).toBe('/p/spacewave-core/export/u/7/so/space%2Fgit')
    expect(props?.objectInfo?.info?.case).toBe('worldObjectInfo')
    expect(props?.objectInfo?.info?.value?.objectKey).toBe('repo/demo')
    expect(props?.objectInfo?.info?.value?.objectType).toBeUndefined()
    expect(props?.path).toBe('/')
    expect(props?.stateNamespace).toEqual(['objectViewer', 'repo/demo'])
  })

  it('passes the current space object type to the viewer when space state has it', () => {
    h.spaceContext.spaceState = {
      worldContents: {
        objects: [{ objectKey: 'repo/demo', objectType: 'git/repo' }],
      },
    }

    render(<SpaceObjectContainer />)

    const props = h.objectViewer.mock.calls[0]?.[0]
    expect(props?.objectInfo?.info?.value?.objectType).toBe('git/repo')
  })

  it('uses the quickstart handoff object type before space state is ready', () => {
    h.getQuickstartInitialObjectHandoff.mockReturnValue({
      objectKey: 'repo/demo',
      objectType: 'unixfs/fs-node',
    })

    render(<SpaceObjectContainer />)

    expect(h.getQuickstartInitialObjectHandoff).toHaveBeenCalledWith(
      7,
      'space/git',
      'repo/demo',
    )
    const props = h.objectViewer.mock.calls[0]?.[0]
    expect(props?.objectInfo?.info?.value?.objectType).toBe('unixfs/fs-node')
  })

  it('keeps child viewer navigation scoped under the current object key', () => {
    render(<SpaceObjectContainer />)

    const props = h.objectViewer.mock.calls[0]?.[0]
    props?.onNavigate?.({ path: 'docs/readme.md' })

    expect(h.navigateToSubPath).toHaveBeenCalledWith(
      'repo/demo/-/docs/readme.md',
    )
  })
})

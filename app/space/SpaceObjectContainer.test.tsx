import { render, within } from '@testing-library/react'
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
  onNavigate?: (to: {
    path: string
    history?: 'local'
    navigation?: 'local'
  }) => void
  stateNamespace?: string[]
}

const h = vi.hoisted(() => ({
  objectViewer: vi.fn((_props: CapturedObjectViewerProps) => null),
  navigate: vi.fn(),
  navigateToRoot: vi.fn(),
  navigateToSubPath: vi.fn(),
  getQuickstartInitialObjectHandoff: vi.fn(),
  worldObjectType: '',
  metadataError: null as Error | null,
  viewerMounts: 0,
  spaceContext: {
    spaceId: 'space/git',
    objectKey: 'repo/demo',
    objectPath: '',
    spaceWorldResource: { value: null, loading: false, error: null },
  },
  spaceContentsResource: {
    value: {},
    loading: false,
    error: null,
    retry: vi.fn(),
  },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  resolvePath: (currentPath: string, to: { path: string }) =>
    to.path.startsWith('/')
      ? to.path
      : `${currentPath.replace(/\/$/, '')}/${to.path}`,
  useNavigate: () => h.navigate,
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', async () => {
  const { useEffect } = await import('react')
  return {
    ObjectViewer: (props: CapturedObjectViewerProps) => {
      h.objectViewer(props)
      useEffect(() => {
        h.viewerMounts++
      }, [])
      return <div data-testid="object-viewer" />
    },
  }
})

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContentsContext: {
    useContext: () => h.spaceContentsResource,
  },
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

vi.mock('@s4wave/web/hooks/useObjectMetadata.js', () => ({
  useObjectMetadata: () => {
    const entry = h.worldObjectType ? { typeId: h.worldObjectType } : null
    if (h.metadataError) {
      return {
        value: null,
        loading: false,
        error: h.metadataError,
        last: entry,
        retry: vi.fn(),
      }
    }
    return { value: entry, loading: false, error: null, retry: vi.fn() }
  },
}))

vi.mock('@s4wave/app/quickstart/session-handoff.js', () => ({
  getQuickstartInitialObjectHandoff: h.getQuickstartInitialObjectHandoff,
}))

describe('SpaceObjectContainer', () => {
  beforeEach(() => {
    h.objectViewer.mockClear()
    h.navigate.mockClear()
    h.navigateToRoot.mockClear()
    h.navigateToSubPath.mockClear()
    h.getQuickstartInitialObjectHandoff.mockReset()
    h.worldObjectType = ''
    h.metadataError = null
    h.viewerMounts = 0
    h.spaceContext = {
      spaceId: 'space/git',
      objectKey: 'repo/demo',
      objectPath: '',
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

  it('keeps the viewer mounted while a type read fails and recovers', () => {
    h.worldObjectType = 'git/repo'
    const { container, rerender } = render(<SpaceObjectContainer />)
    const view = within(container)

    h.metadataError = new Error('world closed')
    rerender(<SpaceObjectContainer />)
    expect(view.getByRole('alert').textContent).toContain(
      'Could not read the object: world closed',
    )
    expect(view.getByTestId('object-viewer')).toBeDefined()

    h.metadataError = null
    rerender(<SpaceObjectContainer />)
    expect(view.queryByRole('alert')).toBeNull()
    expect(h.viewerMounts).toBe(1)
  })

  it('passes the World object type to the viewer', () => {
    h.worldObjectType = 'git/repo'

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
      '',
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

  it.each([
    ['/', 'repo/demo'],
    ['/docs/readme.md', 'repo/demo/-/docs/readme.md'],
  ])(
    'keeps local history target %s scoped under the current object key',
    (path, expectedSubpath) => {
      render(<SpaceObjectContainer />)

      const props = h.objectViewer.mock.calls[0]?.[0]
      props?.onNavigate?.({ path, history: 'local' })

      expect(h.navigateToSubPath).toHaveBeenCalledWith(expectedSubpath)
      expect(h.navigate).not.toHaveBeenCalled()
    },
  )

  it('keeps an explicit local navigation target under the current object key', () => {
    render(<SpaceObjectContainer />)

    const props = h.objectViewer.mock.calls[0]?.[0]
    props?.onNavigate?.({ path: '/', navigation: 'local' })

    expect(h.navigateToSubPath).toHaveBeenCalledWith('repo/demo')
    expect(h.navigate).not.toHaveBeenCalled()
  })

  it('forwards absolute viewer routes to the app router', () => {
    render(<SpaceObjectContainer />)

    const props = h.objectViewer.mock.calls[0]?.[0]
    props?.onNavigate?.({ path: '/login' })

    expect(h.navigate).toHaveBeenCalledWith({ path: '/login' })
    expect(h.navigateToSubPath).not.toHaveBeenCalled()
  })
})

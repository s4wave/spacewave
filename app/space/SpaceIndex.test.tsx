import type { DependencyList } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import type { WorldQuery } from '@s4wave/sdk/world/world-query.js'
import type { TypedObject } from '@s4wave/web/test/world-query.js'

const h = vi.hoisted(() => ({ objects: [] as TypedObject[] }))

const mockRootResource = vi.hoisted(() => ({ value: null }))

const mockOpenCommand = vi.hoisted(() => vi.fn())
const mockUseSpaceContainer = vi.hoisted(() => vi.fn())
const mockToastSuccess = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: mockUseSpaceContainer,
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  RootContext: {
    useContext: () => mockRootResource,
  },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: mockToastSuccess,
  },
}))

vi.mock('@s4wave/web/hooks/useWorldQuery.js', async () => {
  const { listingWorld, useFakeWorldQuery } =
    await import('@s4wave/web/test/world-query.js')
  const world = listingWorld(() => h.objects)
  return {
    useWorldQuery: <T,>(
      _world: unknown,
      query: WorldQuery<T>,
      deps: DependencyList,
    ) => useFakeWorldQuery(world, query, deps),
  }
})

import { SpaceIndex } from './SpaceIndex.js'

describe('SpaceIndex', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('redirects to the configured index path through the space navigator', async () => {
    const navigateToSubPath = vi.fn()
    const applyWorldOp = vi.fn()
    h.objects = [{ objectKey: 'files', objectType: 'unixfs/fs-node' }]
    mockUseSpaceContainer.mockReturnValue({
      spaceWorldResource: {},
      spaceState: {
        settings: { indexPath: 'files' },
      },
      spaceWorld: { applyWorldOp },
      navigateToSubPath,
    })

    render(<SpaceIndex />)

    await waitFor(() => {
      expect(navigateToSubPath).toHaveBeenCalledWith('files')
    })
    expect(navigateToSubPath).toHaveBeenCalledTimes(1)
    expect(applyWorldOp).not.toHaveBeenCalled()
  })

  it('repairs a stale index path to the matching numbered object', async () => {
    const navigateToSubPath = vi.fn()
    const applyWorldOp = vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false })
    h.objects = [
      { objectKey: 'files-1', objectType: 'unixfs/fs-node' },
      { objectKey: 'settings', objectType: 'space/settings' },
    ]
    mockUseSpaceContainer.mockReturnValue({
      spaceWorldResource: {},
      spaceState: {
        settings: { indexPath: 'files', pluginIds: ['spacewave-app'] },
      },
      spaceWorld: { applyWorldOp },
      navigateToSubPath,
    })

    render(<SpaceIndex />)

    await waitFor(() => {
      expect(navigateToSubPath).toHaveBeenCalledWith('files-1')
    })
    await waitFor(() => {
      expect(applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const opData = applyWorldOp.mock.calls[0]?.[1] as Uint8Array | undefined
    expect(opData).toBeDefined()
    const op = SetSpaceIndexPathOp.fromBinary(opData)
    expect(op.indexPath).toBe('files-1')
    expect(op.expectedIndexPath).toBe('files')

    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Default object updated to files-1',
    )
  })

  it('renders the empty state when no index path is configured', async () => {
    h.objects = []
    mockUseSpaceContainer.mockReturnValue({
      spaceWorldResource: {},
      spaceState: {
        settings: { indexPath: '' },
      },
      spaceWorld: { applyWorldOp: vi.fn() },
      navigateToSubPath: vi.fn(),
    })

    render(<SpaceIndex />)

    expect(await screen.findByText('Empty Space')).toBeDefined()
  })

  it('renders the object list when objects exist without an index path', async () => {
    const navigateToObjects = vi.fn()
    h.objects = [{ objectKey: 'files', objectType: 'unixfs/fs-node' }]
    mockUseSpaceContainer.mockReturnValue({
      spaceWorldResource: {},
      spaceState: {
        settings: {},
      },
      spaceWorld: { applyWorldOp: vi.fn() },
      navigateToObjects,
      navigateToSubPath: vi.fn(),
      canDeleteObjects: false,
    })

    render(<SpaceIndex />)

    await waitFor(() => {
      expect(screen.getByText('files')).toBeDefined()
    })
    expect(
      screen.getByText('Select an object to view, or add a new one.'),
    ).toBeDefined()
    expect(screen.queryByText('Empty Space')).toBeNull()

    fireEvent.doubleClick(screen.getByRole('treeitem', { name: /files/i }))
    expect(navigateToObjects).toHaveBeenCalledWith(['files'])

    fireEvent.click(screen.getByRole('button', { name: 'Add an object' }))
    expect(mockOpenCommand).toHaveBeenCalledWith('spacewave.create-object')
  })

  it('renders the empty state without repair when a stale index has no visible replacement', async () => {
    const navigateToSubPath = vi.fn()
    const applyWorldOp = vi.fn()
    h.objects = [{ objectKey: 'settings', objectType: 'space/settings' }]
    mockUseSpaceContainer.mockReturnValue({
      spaceWorldResource: {},
      spaceState: {
        settings: { indexPath: 'missing' },
      },
      spaceWorld: { applyWorldOp },
      navigateToSubPath,
    })

    render(<SpaceIndex />)

    expect(await screen.findByText('Empty Space')).toBeDefined()
    expect(navigateToSubPath).not.toHaveBeenCalled()
    expect(applyWorldOp).not.toHaveBeenCalled()
  })
})

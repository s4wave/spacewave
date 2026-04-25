import React from 'react'
import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  fireEvent,
  render,
  screen,
  cleanup,
  waitFor,
} from '@testing-library/react'
import { SpaceObjectBrowser } from './SpaceObjectBrowser.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SpaceState } from '@s4wave/sdk/space/space.pb.js'
import type { EngineWorldState } from '@s4wave/sdk/world/engine-state.js'
import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    error: vi.fn(),
  },
}))

vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}))

const mockOpenCommand = vi.fn()

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/ui/DropdownMenu.js', () => ({
  DropdownMenu: ({
    children,
    open,
  }: {
    children: React.ReactNode
    open?: boolean
  }) =>
    open === false ? null : (
      <div data-testid={open ? 'context-menu' : undefined}>{children}</div>
    ),
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    onClick,
    variant,
    disabled,
  }: {
    children: React.ReactNode
    onClick?: () => void
    variant?: string
    disabled?: boolean
  }) => (
    <button onClick={onClick} data-variant={variant} disabled={disabled}>
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
}))

describe('SpaceObjectBrowser', () => {
  const mockNavigateToObjects = vi.fn()

  const mockSpaceWorld = {
    applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
    deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
    renameObject: vi.fn().mockResolvedValue({ release: vi.fn() }),
    getEngine: vi.fn(),
  } as unknown as EngineWorldState

  const mockWorldResource: Resource<EngineWorldState> = {
    value: mockSpaceWorld,
    loading: false,
    error: null,
    retry: vi.fn(),
  }

  const mockSpaceState: SpaceState = {
    ready: true,
    worldContents: {
      objects: [
        { objectKey: 'object-layout/main', objectType: 'alpha/object-layout' },
        { objectKey: 'files', objectType: 'unixfs/fs-node' },
        { objectKey: 'canvas-1', objectType: 'canvas' },
        { objectKey: 'settings', objectType: 'space/settings' },
      ],
    },
    settings: { indexPath: 'object-layout/main' },
  }

  function renderBrowser(
    spaceState: SpaceState = mockSpaceState,
    props?: React.ComponentProps<typeof SpaceObjectBrowser>,
  ) {
    return render(
      <SpaceContainerContext.Provider
        spaceId="test-space"
        spaceState={spaceState}
        spaceWorldResource={mockWorldResource}
        spaceWorld={mockSpaceWorld}
        navigateToRoot={vi.fn()}
        navigateToObjects={mockNavigateToObjects}
        buildObjectUrls={vi.fn()}
        navigateToSubPath={vi.fn()}
      >
        <SpaceObjectBrowser {...props} />
      </SpaceContainerContext.Provider>,
    )
  }

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders "Objects" heading', () => {
    renderBrowser()
    expect(screen.getByText('Objects')).toBeDefined()
  })

  it('omits the outer heading when embedded', () => {
    renderBrowser(mockSpaceState, { embedded: true })
    expect(screen.queryByText('Objects')).toBeNull()
    expect(screen.queryByText('3 objects')).toBeNull()
  })

  it('shows correct object count excluding hidden types', () => {
    renderBrowser()
    // 4 objects total, but space/settings is hidden, so count is 3
    expect(screen.getByText('(3)')).toBeDefined()
  })

  it('opens the command palette from the create button', () => {
    renderBrowser()
    const buttons = screen.getAllByRole('button')
    fireEvent.click(buttons[0])
    expect(mockOpenCommand).toHaveBeenCalledWith('spacewave.create-object')
  })

  it('renders tree with top-level nodes', async () => {
    renderBrowser()
    await waitFor(() => {
      expect(screen.getByText('canvas-1')).toBeDefined()
      expect(screen.getByText('files')).toBeDefined()
      expect(screen.getByText('object-layout')).toBeDefined()
    })
  })

  it('does not render the settings object in the tree', async () => {
    renderBrowser()
    await waitFor(() => {
      expect(screen.getByText('canvas-1')).toBeDefined()
    })
    // "settings" objectKey with type "space/settings" should be filtered out
    // It would appear as a top-level node named "settings" if not hidden
    expect(screen.queryByText('settings')).toBeNull()
  })

  it('hides the fully-qualified SpaceSettings type from the tree and count', async () => {
    const fullyQualifiedSettingsState: SpaceState = {
      ready: true,
      worldContents: {
        objects: [
          {
            objectKey: 'settings',
            objectType:
              'github.com/s4wave/spacewave/core/space/world.SpaceSettings',
          },
          { objectKey: 'canvas-1', objectType: 'canvas' },
        ],
      },
      settings: {},
    }
    renderBrowser(fullyQualifiedSettingsState)
    expect(screen.getByText('(1)')).toBeDefined()
    await waitFor(() => {
      expect(screen.getByText('canvas-1')).toBeDefined()
    })
    expect(screen.queryByText('settings')).toBeNull()
  })

  it('renders virtual folder nodes for prefix paths', async () => {
    renderBrowser()
    await waitFor(() => {
      expect(screen.getByText('canvas-1')).toBeDefined()
      expect(screen.getByText('files')).toBeDefined()
      expect(screen.getByText('object-layout')).toBeDefined()
    })
  })

  it('does not render context menu when menuState is null', () => {
    renderBrowser()
    expect(screen.queryByTestId('context-menu')).toBeNull()
  })

  it('renames an object key from the context menu', async () => {
    renderBrowser()

    const node = await screen.findByText('files')
    fireEvent.contextMenu(node, { clientX: 120, clientY: 140 })
    fireEvent.click(screen.getByRole('button', { name: /rename object key/i }))

    fireEvent.change(screen.getByLabelText('New object key'), {
      target: { value: 'files-main' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => {
      expect(mockSpaceWorld.renameObject).toHaveBeenCalledWith(
        'files',
        'files-main',
        { descendants: true },
      )
    })
  })

  it('renames descendant object keys with their parent prefix', async () => {
    const gitState: SpaceState = {
      ready: true,
      worldContents: {
        objects: [
          { objectKey: 'repo-1', objectType: 'git/repo' },
          { objectKey: 'repo-1/workdir', objectType: 'unixfs/fs-node' },
          { objectKey: 'repo-1/worktree', objectType: 'git/worktree' },
        ],
      },
      settings: { indexPath: 'repo-1' },
    }
    renderBrowser(gitState)

    const node = await screen.findByText('repo-1')
    fireEvent.contextMenu(node, { clientX: 120, clientY: 140 })
    fireEvent.click(screen.getByRole('button', { name: /rename object key/i }))

    fireEvent.change(screen.getByLabelText('New object key'), {
      target: { value: 'myrepo' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => {
      expect(mockSpaceWorld.renameObject).toHaveBeenCalledTimes(1)
    })
    expect(mockSpaceWorld.renameObject).toHaveBeenCalledWith(
      'repo-1',
      'myrepo',
      { descendants: true },
    )
    const settingsData = vi.mocked(mockSpaceWorld.applyWorldOp).mock
      .calls[0]?.[1]
    expect(settingsData).toBeInstanceOf(Uint8Array)
    const settingsOp = SetSpaceSettingsOp.fromBinary(settingsData as Uint8Array)
    expect(settingsOp.settings?.indexPath).toBe('myrepo')
  })

  it('anchors the context menu in document.body at the click position', async () => {
    renderBrowser()

    fireEvent.click(
      screen.getByRole('button', { name: /expand object-layout/i }),
    )

    const node = await screen.findByText('main')
    fireEvent.contextMenu(node, { clientX: 120, clientY: 140 })

    const anchor = document.body.querySelector(
      '[data-slot="dropdown-menu-ghost-anchor"]',
    )
    if (!(anchor instanceof HTMLDivElement)) {
      throw new Error('expected dropdown menu ghost anchor')
    }

    expect(anchor.style.position).toBe('fixed')
    expect(anchor.style.left).toBe('120px')
    expect(anchor.style.top).toBe('140px')
  })

  it('shows zero count when no objects exist', () => {
    const emptyState: SpaceState = {
      ready: true,
      worldContents: { objects: [] },
      settings: {},
    }
    renderBrowser(emptyState)
    expect(screen.getByText('(0)')).toBeDefined()
  })

  it('shows "No objects" placeholder when object list is empty', async () => {
    const emptyState: SpaceState = {
      ready: true,
      worldContents: { objects: [] },
      settings: {},
    }
    renderBrowser(emptyState)
    await waitFor(() => {
      expect(screen.getByText('No objects')).toBeDefined()
    })
  })
})

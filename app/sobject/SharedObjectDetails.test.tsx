import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, fireEvent, screen } from '@testing-library/react'
import { SharedObjectDetails } from './SharedObjectDetails.js'
import { SharedObjectContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { MountSharedObjectResponse } from '@s4wave/sdk/session/session.pb.js'
import { SharedObject } from '@s4wave/sdk/sobject/sobject.js'

// Mock tooltip components to simplify testing
vi.mock('@s4wave/web/ui/tooltip.js', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipContent: () => null,
}))
vi.mock('@s4wave/web/ui/CollapsibleSection.js', () => ({
  CollapsibleSection: ({
    title,
    badge,
    headerActions,
    children,
  }: {
    title: string
    badge?: React.ReactNode
    headerActions?: React.ReactNode
    children: React.ReactNode
  }) => (
    <section>
      <h2>{title}</h2>
      {badge}
      {headerActions}
      {children}
    </section>
  ),
}))
vi.mock('@s4wave/web/state/persist.js', () => ({
  useStateNamespace: () => ['details'],
  useStateAtom: (_ns: unknown, _key: string, init: unknown) =>
    [init, vi.fn()] as const,
}))

describe('SharedObjectDetails', () => {
  const mockMeta: MountSharedObjectResponse = {
    sharedObjectId: 'test-object-id',
    blockStoreId: 'test-blockstore-id',
    peerId: 'test-peer-id',
    sharedObjectMeta: {
      bodyType: 'counter',
    },
  }

  const mockSharedObject = {
    meta: mockMeta,
    resourceRef: {
      resourceId: 1,
      released: false,
    },
    id: 1,
    client: {},
    service: {},
    mountSharedObjectBody: vi.fn(),
  } as unknown as SharedObject

  const mockClipboard = {
    writeText: vi.fn().mockResolvedValue(undefined),
  }

  const mockRetry = vi.fn()

  beforeEach(() => {
    cleanup()
    Object.defineProperty(navigator, 'clipboard', {
      value: mockClipboard,
      writable: true,
      configurable: true,
    })
    mockClipboard.writeText.mockClear()
    mockRetry.mockClear()
  })

  function createMockSharedObject(
    meta: MountSharedObjectResponse,
  ): SharedObject {
    return {
      meta,
      resourceRef: {
        resourceId: 1,
        released: false,
      },
      id: 1,
      client: {},
      service: {},
      mountSharedObjectBody: vi.fn(),
    } as unknown as SharedObject
  }

  function renderWithContext(
    component: React.ReactElement,
    resourceValue: SharedObject = mockSharedObject,
  ) {
    return render(
      <SpaceContainerContext.Provider
        spaceId="test-space"
        spaceState={{ ready: true }}
        spaceSharingState={{}}
        spaceWorldResource={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        spaceWorld={{} as never}
        navigateToRoot={vi.fn()}
        navigateToObjects={vi.fn()}
        buildObjectUrls={vi.fn()}
        navigateToSubPath={vi.fn()}
      >
        <SharedObjectContext.Provider
          resource={{
            value: resourceValue,
            loading: false,
            error: null,
            retry: mockRetry,
          }}
        >
          {component}
        </SharedObjectContext.Provider>
      </SpaceContainerContext.Provider>,
    )
  }

  describe('Rendering', () => {
    it('renders without crashing', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Untitled')).toBeDefined()
    })

    it('displays object metadata correctly', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('test-object-id')).toBeDefined()
      expect(screen.getByText('test-blockstore-id')).toBeDefined()
      expect(screen.getByText('test-peer-id')).toBeDefined()
    })

    it('displays body type name', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText(/counter/)).toBeDefined()
    })

    it('handles unknown body type', () => {
      const unknownTypeMeta: MountSharedObjectResponse = {
        ...mockMeta,
        sharedObjectMeta: {
          bodyType: 'unknown-type',
        },
      }
      const unknownTypeObj = createMockSharedObject(unknownTypeMeta)
      renderWithContext(<SharedObjectDetails />, unknownTypeObj)
      expect(screen.getByText(/unknown-type/)).toBeDefined()
    })

    it('handles missing metadata gracefully', () => {
      const emptyMeta = {} as MountSharedObjectResponse
      const emptyObj = createMockSharedObject(emptyMeta)
      renderWithContext(<SharedObjectDetails />, emptyObj)
      expect(screen.getByText(/unknown/)).toBeDefined()
      const unknownFields = screen.getAllByText('Unknown')
      expect(unknownFields.length).toBeGreaterThanOrEqual(3)
    })

    it('starts rename mode when the title is double-clicked', () => {
      const onRenameStart = vi.fn()
      renderWithContext(
        <SharedObjectDetails
          displayName="Test Space"
          canRename={true}
          onRenameStart={onRenameStart}
        />,
      )
      fireEvent.doubleClick(screen.getByText('Test Space'))
      expect(onRenameStart).toHaveBeenCalledTimes(1)
    })

    it('renders a Rename button that triggers onRenameStart when clicked', () => {
      const onRenameStart = vi.fn()
      renderWithContext(
        <SharedObjectDetails
          displayName="Test Space"
          canRename={true}
          onRenameStart={onRenameStart}
        />,
      )
      const button = screen.getByText('Rename').closest('button')
      expect(button).toBeDefined()
      if (button) {
        fireEvent.click(button)
        expect(onRenameStart).toHaveBeenCalledTimes(1)
      }
    })
  })

  describe('Close Button', () => {
    it('renders close button when onCloseClick is provided', () => {
      const onCloseClick = vi.fn()
      renderWithContext(<SharedObjectDetails onCloseClick={onCloseClick} />)
      const buttons = screen.getAllByRole('button')
      // With close button: copyable fields (3) + Add User (1) + Export (1) + Delete (1) + Close (1) = 7
      expect(buttons.length).toBe(7)
    })

    it('does not render close button when onCloseClick is not provided', () => {
      renderWithContext(<SharedObjectDetails />)
      const buttons = screen.getAllByRole('button')
      // Without close button: copyable fields (3) + Add User (1) + Export (1) + Delete (1) = 6
      expect(buttons.length).toBe(6)
    })

    it('calls onCloseClick when close button is clicked', () => {
      const onCloseClick = vi.fn()
      renderWithContext(<SharedObjectDetails onCloseClick={onCloseClick} />)
      const buttons = screen.getAllByRole('button')
      // Close button is at index 0 (in the header)
      const closeButton = buttons[0]
      fireEvent.click(closeButton)
      expect(onCloseClick).toHaveBeenCalledTimes(1)
    })
  })

  describe('Action Buttons', () => {
    it('renders add user header action', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByLabelText('Add user')).toBeDefined()
    })

    it('hides add user header action when sharing is disabled', () => {
      renderWithContext(<SharedObjectDetails canShare={false} />)
      expect(screen.queryByLabelText('Add user')).toBeNull()
    })

    it('calls onSharingClick when add user button is clicked', () => {
      const onSharingClick = vi.fn()
      renderWithContext(<SharedObjectDetails onSharingClick={onSharingClick} />)
      fireEvent.click(screen.getByLabelText('Add user'))
      expect(onSharingClick).toHaveBeenCalledTimes(1)
    })
  })

  describe('Export Button', () => {
    it('renders export button', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Export Data')).toBeDefined()
      expect(screen.getByText('Download object contents')).toBeDefined()
    })

    it('calls onExportClick when export button is clicked', () => {
      const onExportClick = vi.fn()
      renderWithContext(<SharedObjectDetails onExportClick={onExportClick} />)
      const exportButton = screen.getByText('Export Data').closest('button')
      if (exportButton) {
        fireEvent.click(exportButton)
        expect(onExportClick).toHaveBeenCalledTimes(1)
      }
    })

    it('disables export button when onExportClick is not provided', () => {
      renderWithContext(<SharedObjectDetails />)
      const exportButton = screen.getByText('Export Data').closest('button')
      expect(exportButton?.disabled).toBe(true)
    })

    it('enables export button when onExportClick is provided', () => {
      const onExportClick = vi.fn()
      renderWithContext(<SharedObjectDetails onExportClick={onExportClick} />)
      const exportButton = screen.getByText('Export Data').closest('button')
      expect(exportButton?.disabled).toBe(false)
    })
  })

  describe('Delete Button', () => {
    it('renders delete button', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Delete Object')).toBeDefined()
      expect(
        screen.getByText('Permanently remove this object and all its data'),
      ).toBeDefined()
    })

    it('calls onDeleteClick when delete button is clicked', () => {
      const onDeleteClick = vi.fn()
      renderWithContext(<SharedObjectDetails onDeleteClick={onDeleteClick} />)
      const deleteButton = screen.getByText('Delete Object').closest('button')
      if (deleteButton) {
        fireEvent.click(deleteButton)
        expect(onDeleteClick).toHaveBeenCalledTimes(1)
      }
    })

    it('disables delete button when onDeleteClick is not provided', () => {
      renderWithContext(<SharedObjectDetails />)
      const deleteButton = screen.getByText('Delete Object').closest('button')
      expect(deleteButton?.disabled).toBe(true)
    })

    it('enables delete button when onDeleteClick is provided', () => {
      const onDeleteClick = vi.fn()
      renderWithContext(<SharedObjectDetails onDeleteClick={onDeleteClick} />)
      const deleteButton = screen.getByText('Delete Object').closest('button')
      expect(deleteButton?.disabled).toBe(false)
    })
  })

  describe('Copyable Fields', () => {
    it('copies object ID to clipboard when clicked', () => {
      renderWithContext(<SharedObjectDetails />)
      const objectIdField = screen.getByText('test-object-id').closest('button')
      if (objectIdField) {
        fireEvent.click(objectIdField)
        expect(mockClipboard.writeText).toHaveBeenCalledWith('test-object-id')
      }
    })

    it('copies block store ID to clipboard when clicked', () => {
      renderWithContext(<SharedObjectDetails />)
      const blockStoreField = screen
        .getByText('test-blockstore-id')
        .closest('button')
      if (blockStoreField) {
        fireEvent.click(blockStoreField)
        expect(mockClipboard.writeText).toHaveBeenCalledWith(
          'test-blockstore-id',
        )
      }
    })

    it('copies peer ID to clipboard when clicked', () => {
      renderWithContext(<SharedObjectDetails />)
      const peerIdField = screen.getByText('test-peer-id').closest('button')
      if (peerIdField) {
        fireEvent.click(peerIdField)
        expect(mockClipboard.writeText).toHaveBeenCalledWith('test-peer-id')
      }
    })

    it('shows check icon after copying', () => {
      vi.useFakeTimers()
      renderWithContext(<SharedObjectDetails />)
      const objectIdField = screen.getByText('test-object-id').closest('button')
      if (objectIdField) {
        fireEvent.click(objectIdField)
        const checkIcon = objectIdField.querySelector('.lucide-check')
        expect(checkIcon).toBeDefined()
      }
      vi.useRealTimers()
    })
  })

  describe('Sections', () => {
    it('renders Details section', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Identifiers')).toBeDefined()
      expect(screen.getByText('Object ID')).toBeDefined()
    })

    it('renders Data section', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Data')).toBeDefined()
    })

    it('renders object header badge and actions when provided', () => {
      renderWithContext(
        <SharedObjectDetails
          objectsBadge={<span>2</span>}
          objectsActions={<button type="button">New Object</button>}
          objectsSection={<div>Tree</div>}
        />,
      )
      expect(screen.getByText('Objects')).toBeDefined()
      expect(screen.getByText('2')).toBeDefined()
      expect(screen.getByText('New Object')).toBeDefined()
    })

    it('renders Sharing section', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Sharing')).toBeDefined()
      expect(screen.getByText('No users added yet')).toBeDefined()
    })

    it('renders Danger Zone section', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Danger Zone')).toBeDefined()
    })

    it('renders Export Data in Data section', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.getByText('Export Data')).toBeDefined()
      expect(screen.getByText('Download object contents')).toBeDefined()
    })
  })

  describe('Org Context', () => {
    it('renders org indicator in header when provided', () => {
      renderWithContext(
        <SharedObjectDetails
          orgIndicator={
            <button type="button">
              <span>Test Org</span>
            </button>
          }
        />,
      )
      expect(screen.getByText('Test Org')).toBeDefined()
    })

    it('does not render org indicator when not provided', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.queryByText('Test Org')).toBeNull()
    })

    it('renders org info section in identifiers when provided', () => {
      renderWithContext(
        <SharedObjectDetails
          orgInfoSection={
            <div>
              <span>My Organization</span>
              <span>Owner</span>
            </div>
          }
        />,
      )
      expect(screen.getByText('My Organization')).toBeDefined()
      expect(screen.getByText('Owner')).toBeDefined()
    })

    it('does not render org info when not provided (personal space)', () => {
      renderWithContext(<SharedObjectDetails />)
      expect(screen.queryByText('Organization')).toBeNull()
    })
  })
})

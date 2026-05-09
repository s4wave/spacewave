import React from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'

import { DeleteSpaceEscapeHatchDialog } from './DeleteSpaceEscapeHatchDialog.js'

const mockUseWatchStateRpc = vi.hoisted(() => vi.fn())
const mockDeleteSpace = vi.hoisted(() => vi.fn())

const watchState: { spacesList: unknown[] } = { spacesList: [] }

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: (...args: unknown[]) => {
    const state: unknown = mockUseWatchStateRpc(...args)
    return state
  },
}))

vi.mock('@s4wave/web/ui/dialog.js', () => ({
  Dialog: ({
    open,
    children,
  }: {
    open: boolean
    children?: React.ReactNode
    onOpenChange?: (open: boolean) => void
  }) => (open ? <div>{children}</div> : null),
  DialogContent: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogDescription: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogFooter: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

function buildHealth(status: SharedObjectHealthStatus): SharedObjectHealth {
  return {
    status,
    error: '',
  }
}

function renderDialog() {
  const session = {
    deleteSpace: mockDeleteSpace,
  }
  return render(
    <DeleteSpaceEscapeHatchDialog
      open={true}
      onOpenChange={vi.fn()}
      session={session as never}
    />,
  )
}

describe('DeleteSpaceEscapeHatchDialog', () => {
  beforeEach(() => {
    cleanup()
    mockDeleteSpace.mockReset()
    mockUseWatchStateRpc.mockReset()
    watchState.spacesList = [
      {
        entry: {
          ref: {
            providerResourceRef: {
              id: 'space-1',
            },
          },
        },
        spaceMeta: {
          name: 'Broken Space',
        },
      },
      {
        entry: {
          ref: {
            providerResourceRef: {
              id: 'space-2',
            },
          },
        },
        spaceMeta: {},
      },
    ]
    mockUseWatchStateRpc.mockImplementation(
      (_watch: unknown, req: { sharedObjectId?: string } | null) => {
        if (req && 'sharedObjectId' in req && req.sharedObjectId) {
          return {
            health: buildHealth(SharedObjectHealthStatus.CLOSED),
          }
        }
        return {
          spacesList: watchState.spacesList,
        }
      },
    )
  })

  afterEach(() => {
    cleanup()
  })

  it('lists spaces from the session resources list', () => {
    renderDialog()

    expect(screen.getAllByText('Broken Space').length).toBeGreaterThan(0)
    expect(screen.getByText('Unnamed space')).toBeDefined()
    expect(screen.getByText('space-1')).toBeDefined()
    expect(screen.getByText('space-2')).toBeDefined()
  })

  it('gates deletion behind selection, acknowledgment, and typed confirmation', () => {
    renderDialog()

    const continueButtons = screen.getAllByRole('button', {
      name: 'Review space deletion',
    })
    expect(continueButtons[0]?.hasAttribute('disabled')).toBe(true)

    fireEvent.click(screen.getByRole('radio', { name: /Broken Space/i }))
    expect(continueButtons[0]?.hasAttribute('disabled')).toBe(false)

    fireEvent.click(continueButtons[0])

    const warningContinue = screen.getByRole('button', {
      name: 'Confirm space deletion',
    })
    expect(warningContinue.hasAttribute('disabled')).toBe(true)

    fireEvent.click(screen.getByLabelText('Confirm delete is permanent'))
    expect(warningContinue.hasAttribute('disabled')).toBe(false)

    fireEvent.click(warningContinue)

    const deleteButton = screen.getByRole('button', { name: 'Delete Space' })
    expect(deleteButton.hasAttribute('disabled')).toBe(true)

    fireEvent.change(screen.getByLabelText('Confirm space name or id'), {
      target: { value: 'Broken Space' },
    })

    expect(deleteButton.hasAttribute('disabled')).toBe(false)
  })

  it('calls deleteSpace with the selected shared object id', async () => {
    mockDeleteSpace.mockResolvedValue(undefined)
    renderDialog()

    fireEvent.click(screen.getByRole('radio', { name: /Broken Space/i }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Review space deletion' }),
    )
    fireEvent.click(screen.getByLabelText('Confirm delete is permanent'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm space deletion' }),
    )
    fireEvent.change(screen.getByLabelText('Confirm space name or id'), {
      target: { value: 'Broken Space' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Delete Space' }))

    await waitFor(() => {
      expect(mockDeleteSpace).toHaveBeenCalledWith('space-1')
    })
  })

  it('keeps final-step content while the selected space disappears during delete', () => {
    const deleteControl: { resolve?: () => void } = {}
    mockDeleteSpace.mockReturnValue(
      new Promise<void>((resolve) => {
        deleteControl.resolve = resolve
      }),
    )
    const view = renderDialog()

    fireEvent.click(screen.getByRole('radio', { name: /Broken Space/i }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Review space deletion' }),
    )
    fireEvent.click(screen.getByLabelText('Confirm delete is permanent'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm space deletion' }),
    )
    fireEvent.change(screen.getByLabelText('Confirm space name or id'), {
      target: { value: 'Broken Space' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Delete Space' }))

    watchState.spacesList = []
    view.rerender(
      <DeleteSpaceEscapeHatchDialog
        open={true}
        onOpenChange={vi.fn()}
        session={{ deleteSpace: mockDeleteSpace } as never}
      />,
    )

    expect(screen.getAllByText('Broken Space').length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: 'Deleting...' })).toBeDefined()

    deleteControl.resolve?.()
  })

  it('shows mutation errors without closing the modal', async () => {
    mockDeleteSpace.mockRejectedValue(new Error('delete failed'))
    renderDialog()

    fireEvent.click(screen.getByRole('radio', { name: /Broken Space/i }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Review space deletion' }),
    )
    fireEvent.click(screen.getByLabelText('Confirm delete is permanent'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm space deletion' }),
    )
    fireEvent.change(screen.getByLabelText('Confirm space name or id'), {
      target: { value: 'Broken Space' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Delete Space' }))

    await waitFor(() => {
      expect(screen.getByText('delete failed')).toBeDefined()
    })
    expect(screen.getByText('Delete a Space')).toBeDefined()
  })
})

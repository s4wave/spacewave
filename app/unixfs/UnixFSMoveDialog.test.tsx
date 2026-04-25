import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { UnixFSMoveDialog } from './UnixFSMoveDialog.js'
import type { UnixFSMoveItem } from './move.js'

function buildDisposableHandle<T extends object>(value: T) {
  return {
    [Symbol.dispose]: () => undefined,
    ...value,
  }
}

describe('UnixFSMoveDialog', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('disables move for the current parent folder and enables a valid destination', async () => {
    const rootHandle = {
      clone: vi.fn().mockResolvedValue(
        buildDisposableHandle({
          readdirAll: vi.fn().mockResolvedValue([
            { name: 'archive', isDir: true },
            { name: 'docs', isDir: true },
          ]),
          lookup: vi.fn((name: string) =>
            buildDisposableHandle({
              readdirAll: vi.fn().mockResolvedValue([]),
              lookup: vi.fn(),
              getPath: () => name,
            }),
          ),
        }),
      ),
    } as never
    const onConfirm = vi.fn().mockResolvedValue(undefined)
    const moveItems: UnixFSMoveItem[] = [
      { id: 'file', name: 'file.txt', path: '/docs/file.txt', isDir: false },
      { id: 'logo', name: 'logo.png', path: '/docs/logo.png', isDir: false },
    ]

    render(
      <UnixFSMoveDialog
        rootHandle={rootHandle}
        moveItems={moveItems}
        onOpenChange={vi.fn()}
        onConfirm={onConfirm}
      />,
    )

    await screen.findByText('archive')

    fireEvent.click(screen.getByText('docs'))
    expect(
      screen.getByText('The selected items are already in that folder.'),
    ).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Move' }).hasAttribute('disabled'),
    ).toBe(true)

    fireEvent.click(screen.getByText('archive'))
    await waitFor(() => {
      expect(
        screen.queryByText('The selected items are already in that folder.'),
      ).toBeNull()
    })

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'Move' }))
    })

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith('/archive')
    })
  })
})

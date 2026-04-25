import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useCallback, useRef, useState, useMemo } from 'react'
import { FileList } from './FileList.js'
import type { FileEntry } from './types.js'
import type { RenderEntryCallback } from './FileListEntry.js'

const mockEntries: FileEntry[] = [
  { id: '1', name: 'Documents', isDir: true },
  { id: '2', name: 'hello.txt', isDir: false },
  { id: '3', name: 'README.md', isDir: false },
]

function getRenameInput(): HTMLInputElement {
  const input = screen.getByTestId('rename-input')
  if (!(input instanceof HTMLInputElement)) {
    throw new Error('expected rename input')
  }
  return input
}

// RenameTestHarness wraps FileList with inline rename logic matching
// the real UnixFSBrowser implementation: uncontrolled input with a ref,
// confirm/cancel buttons, blur-to-cancel with relatedTarget guard.
function RenameTestHarness({
  onConfirm,
  onCancel,
  initialRenameId,
}: {
  onConfirm: (oldName: string, newName: string) => void
  onCancel: () => void
  initialRenameId?: string
}) {
  const [renamingId, setRenamingId] = useState<string | null>(
    initialRenameId ?? null,
  )
  const renameRef = useRef('')

  const startRename = useCallback((entry: FileEntry) => {
    renameRef.current = entry.name
    setRenamingId(entry.id)
  }, [])

  const confirmRename = useCallback(() => {
    if (!renamingId) return
    const entry = mockEntries.find((e) => e.id === renamingId)
    if (!entry) return
    const newName = renameRef.current.trim()
    if (newName && newName !== entry.name) {
      onConfirm(entry.name, newName)
    }
    setRenamingId(null)
  }, [renamingId, onConfirm])

  const cancelRename = useCallback(() => {
    setRenamingId(null)
    onCancel()
  }, [onCancel])

  const renderEntry: RenderEntryCallback | undefined = useMemo(() => {
    if (!renamingId) return undefined
    return ({ entry, defaultNode }) => {
      if (entry.id !== renamingId) return defaultNode
      return (
        <div
          className="rename-actions flex flex-1 items-center gap-0.5"
          onClick={(e) => e.stopPropagation()}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <input
            autoFocus
            data-testid="rename-input"
            className="flex-1 rounded border px-1 text-xs"
            defaultValue={renameRef.current}
            onChange={(e) => {
              renameRef.current = e.target.value
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                confirmRename()
              }
              if (e.key === 'Escape') {
                e.preventDefault()
                cancelRename()
              }
              e.stopPropagation()
            }}
            onBlur={(e) => {
              const related = e.relatedTarget as HTMLElement | null
              if (related?.closest('.rename-actions')) return
              cancelRename()
            }}
          />
          <button
            tabIndex={0}
            data-testid="rename-confirm"
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              confirmRename()
            }}
          >
            Confirm
          </button>
          <button
            tabIndex={0}
            data-testid="rename-cancel"
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              cancelRename()
            }}
          >
            Cancel
          </button>
        </div>
      )
    }
  }, [renamingId, confirmRename, cancelRename])

  return (
    <div>
      <button
        data-testid="trigger-rename"
        onClick={() => {
          const entry = mockEntries.find((e) => e.id === '2')
          if (entry) startRename(entry)
        }}
      >
        Start Rename
      </button>
      <FileList entries={mockEntries} renderEntry={renderEntry} />
    </div>
  )
}

describe('FileList inline rename', () => {
  afterEach(() => {
    cleanup()
  })

  it('should show rename input when rename is triggered', async () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await userEvent.click(screen.getByTestId('trigger-rename'))

    await waitFor(() => {
      const input = getRenameInput()
      expect(input).toBeTruthy()
      expect(input.value).toBe('hello.txt')
    })
  })

  it('should capture focus in the rename input', async () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await userEvent.click(screen.getByTestId('trigger-rename'))

    await waitFor(() => {
      const input = screen.getByTestId('rename-input')
      expect(document.activeElement).toBe(input)
    })
  })

  it('should keep rename focus when another file row was already focused', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsRow = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    expect(documentsRow).toBeTruthy()

    await user.click(documentsRow!)
    await user.click(screen.getByTestId('trigger-rename'))

    await waitFor(() => {
      const input = screen.getByTestId('rename-input')
      expect(document.activeElement).toBe(input)
    })
  })

  it('should allow typing without losing focus or selecting all', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))

    await waitFor(() => {
      expect(screen.getByTestId('rename-input')).toBeTruthy()
    })

    const input = getRenameInput()
    await user.clear(input)
    await user.type(input, 'renamed.txt')

    expect(input.value).toBe('renamed.txt')
    expect(document.activeElement).toBe(input)
  })

  it('should confirm rename on Enter key', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))

    const input = getRenameInput()
    await user.clear(input)
    await user.type(input, 'newname.txt')
    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith('hello.txt', 'newname.txt')
    })

    expect(screen.queryByTestId('rename-input')).toBeNull()
  })

  it('should cancel rename on Escape key', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))

    const input = getRenameInput()
    await user.clear(input)
    await user.type(input, 'newname.txt')
    await user.keyboard('{Escape}')

    await waitFor(() => {
      expect(onCancel).toHaveBeenCalled()
      expect(onConfirm).not.toHaveBeenCalled()
    })

    expect(screen.queryByTestId('rename-input')).toBeNull()
  })

  it('should confirm rename when clicking the confirm button', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))

    const input = getRenameInput()
    await user.clear(input)
    await user.type(input, 'confirmed.txt')

    await user.click(screen.getByTestId('rename-confirm'))

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith('hello.txt', 'confirmed.txt')
    })

    expect(screen.queryByTestId('rename-input')).toBeNull()
  })

  it('should cancel rename when clicking the cancel button', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))
    await user.click(screen.getByTestId('rename-cancel'))

    await waitFor(() => {
      expect(onCancel).toHaveBeenCalled()
      expect(onConfirm).not.toHaveBeenCalled()
    })

    expect(screen.queryByTestId('rename-input')).toBeNull()
  })

  it('should cancel rename when clicking outside (blur)', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))

    await waitFor(() => {
      expect(screen.getByTestId('rename-input')).toBeTruthy()
    })

    // Click outside the rename area (on a different row)
    const docsRow = screen.getAllByText('Documents')[0].closest('[role="row"]')
    await user.click(docsRow!)

    await waitFor(() => {
      expect(onCancel).toHaveBeenCalled()
      expect(onConfirm).not.toHaveBeenCalled()
    })
  })

  it('should not cancel when unchanged name is confirmed', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />)

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))
    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(onConfirm).not.toHaveBeenCalled()
    })

    expect(screen.queryByTestId('rename-input')).toBeNull()
  })

  it('should not propagate keystrokes to parent handlers', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    const onKeyDown = vi.fn()

    render(
      <div onKeyDown={onKeyDown}>
        <RenameTestHarness onConfirm={onConfirm} onCancel={onCancel} />
      </div>,
    )

    await waitFor(() => {
      expect(screen.getAllByText('hello.txt')[0]).toBeTruthy()
    })

    await user.click(screen.getByTestId('trigger-rename'))

    const input = getRenameInput()
    await user.type(input, 'abc')

    // Keystrokes should not bubble to parent
    expect(onKeyDown).not.toHaveBeenCalled()
  })
})

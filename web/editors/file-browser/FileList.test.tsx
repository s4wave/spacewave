import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  render,
  screen,
  waitFor,
  cleanup,
  fireEvent,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { APP_DRAG_MIME, APP_DRAG_VERSION } from '@s4wave/web/dnd/app-drag.js'
import { DOWNLOAD_URL_DRAG_FORMAT } from '@s4wave/web/dnd/download-url-drag.js'
import { FileList } from './FileList.js'
import { FileEntry } from './types.js'

const mockEntries: FileEntry[] = [
  { id: '1', name: 'Documents', isDir: true },
  { id: '2', name: 'Pictures', isDir: true },
  { id: '3', name: 'file.txt', isDir: false },
  { id: '4', name: 'README.md', isDir: false },
]

const longFilenameEntries: FileEntry[] = [
  { id: '1', name: 'getting-started.md', isDir: false },
  {
    id: '2',
    name: 'this-is-a-very-long-filename-that-should-be-truncated-with-ellipsis.txt',
    isDir: false,
  },
  { id: '3', name: 'short.txt', isDir: false },
]

describe('FileList', () => {
  afterEach(() => {
    cleanup()
  })

  it('should render the file list container and header', () => {
    render(<FileList entries={mockEntries} />)

    expect(screen.getByRole('rowgroup')).toBeTruthy()
    expect(screen.getByText('Name')).toBeTruthy()
    expect(screen.getByText('Date Modified')).toBeTruthy()
    expect(screen.getByText('Size')).toBeTruthy()
  })

  it('should render file entries', async () => {
    render(<FileList entries={mockEntries} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    expect(screen.getAllByText('Pictures')[0]).toBeTruthy()
    expect(screen.getAllByText('file.txt')[0]).toBeTruthy()
    expect(screen.getAllByText('README.md')[0]).toBeTruthy()
  })

  it('should handle single click selection', async () => {
    const user = userEvent.setup()
    render(<FileList entries={mockEntries} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    expect(documentsEntry).toBeTruthy()

    await user.click(documentsEntry!)

    await waitFor(() => {
      expect(documentsEntry?.className).toContain('bg-ui-selected')
    })
  })

  it('should handle double click to open', async () => {
    const user = userEvent.setup()
    const onOpen = vi.fn<(entries: FileEntry[]) => void>()
    render(<FileList entries={mockEntries} onOpen={onOpen} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    await user.dblClick(documentsEntry!)

    await waitFor(
      () => {
        expect(onOpen).toHaveBeenCalledOnce()
        expect(onOpen.mock.calls[0][0]).toHaveLength(1)
        expect(onOpen.mock.calls[0][0][0].name).toBe('Documents')
      },
      { timeout: 1000 },
    )
  })

  it('publishes a provided app-drag envelope on drag start', async () => {
    render(
      <FileList
        entries={mockEntries}
        getDragEnvelope={(entry) => ({
          version: APP_DRAG_VERSION,
          items: [{ id: entry.id, label: entry.name, capabilities: [] }],
        })}
      />,
    )

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    expect(documentsEntry).toBeTruthy()

    const writes = new Map<string, string>()
    const dataTransfer = {
      setData: (format: string, data: string) => {
        writes.set(format, data)
      },
      effectAllowed: '',
    }

    fireEvent.dragStart(documentsEntry!, { dataTransfer })

    expect(writes.get(APP_DRAG_MIME)).toContain('"id":"1"')
  })

  it('passes the active multi-selection into getDragEnvelope for drag start', async () => {
    const user = userEvent.setup()
    const getDragEnvelope = vi.fn(
      (entry: FileEntry, _context: { selectedIds: string[] }) => ({
        version: APP_DRAG_VERSION,
        items: [{ id: entry.id, label: entry.name, capabilities: [] }],
      }),
    )
    render(<FileList entries={mockEntries} getDragEnvelope={getDragEnvelope} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    const fileEntry = screen.getAllByText('file.txt')[0].closest('[role="row"]')
    expect(documentsEntry).toBeTruthy()
    expect(fileEntry).toBeTruthy()

    await user.click(documentsEntry!)
    await user.keyboard('{Control>}')
    await user.click(fileEntry!)
    await user.keyboard('{/Control}')

    const writes = new Map<string, string>()
    const dataTransfer = {
      setData: (format: string, data: string) => {
        writes.set(format, data)
      },
      effectAllowed: '',
    }

    fireEvent.dragStart(fileEntry!, { dataTransfer })

    const dragCalls = getDragEnvelope.mock.calls as Array<
      [FileEntry, { selectedIds: string[] }]
    >
    const matchingCall = dragCalls.find(
      ([entry, context]) =>
        entry.id === '3' &&
        context.selectedIds.includes('1') &&
        context.selectedIds.includes('3'),
    )
    expect(matchingCall).toBeTruthy()
    expect(writes.get(APP_DRAG_MIME)).toContain('"id":"3"')
  })

  it('publishes a provided download drag target alongside app drag data', async () => {
    render(
      <FileList
        entries={mockEntries}
        getDragEnvelope={(entry) => ({
          version: APP_DRAG_VERSION,
          items: [{ id: entry.id, label: entry.name, capabilities: [] }],
        })}
        getDownloadDragTarget={(entry) => ({
          mimeType: 'application/octet-stream',
          filename: entry.name,
          url: `/download/${entry.name}`,
        })}
      />,
    )

    await waitFor(() => {
      expect(screen.getAllByText('file.txt')[0]).toBeTruthy()
    })

    const fileEntry = screen.getAllByText('file.txt')[0].closest('[role="row"]')
    expect(fileEntry).toBeTruthy()

    const writes = new Map<string, string>()
    const dataTransfer = {
      setData: (format: string, data: string) => {
        writes.set(format, data)
      },
      effectAllowed: '',
    }

    fireEvent.dragStart(fileEntry!, { dataTransfer })

    expect(writes.get(APP_DRAG_MIME)).toContain('"id":"3"')
    expect(writes.get(DOWNLOAD_URL_DRAG_FORMAT)).toContain(
      'application/octet-stream:file.txt:',
    )
  })

  it('can publish only download drag data for rows without an app envelope', async () => {
    render(
      <FileList
        entries={mockEntries}
        getDownloadDragTarget={(entry) => ({
          mimeType: 'text/plain',
          filename: entry.name,
          url: `/download/${entry.name}`,
        })}
      />,
    )

    await waitFor(() => {
      expect(screen.getAllByText('README.md')[0]).toBeTruthy()
    })

    const readmeEntry = screen
      .getAllByText('README.md')[0]
      .closest('[role="row"]')
    expect(readmeEntry).toBeTruthy()

    const writes = new Map<string, string>()
    const dataTransfer = {
      setData: (format: string, data: string) => {
        writes.set(format, data)
      },
      effectAllowed: '',
    }

    fireEvent.dragStart(readmeEntry!, { dataTransfer })

    expect(writes.has(APP_DRAG_MIME)).toBe(false)
    expect(writes.get(DOWNLOAD_URL_DRAG_FORMAT)).toContain(
      'text/plain:README.md:',
    )
  })

  it('omits download drag data when the provider returns null', async () => {
    render(
      <FileList
        entries={mockEntries}
        getDragEnvelope={(entry) => ({
          version: APP_DRAG_VERSION,
          items: [{ id: entry.id, label: entry.name, capabilities: [] }],
        })}
        getDownloadDragTarget={() => null}
      />,
    )

    await waitFor(() => {
      expect(screen.getAllByText('file.txt')[0]).toBeTruthy()
    })

    const fileEntry = screen.getAllByText('file.txt')[0].closest('[role="row"]')
    expect(fileEntry).toBeTruthy()

    const writes = new Map<string, string>()
    const dataTransfer = {
      setData: (format: string, data: string) => {
        writes.set(format, data)
      },
      effectAllowed: '',
    }

    fireEvent.dragStart(fileEntry!, { dataTransfer })

    expect(writes.get(APP_DRAG_MIME)).toContain('"id":"3"')
    expect(writes.has(DOWNLOAD_URL_DRAG_FORMAT)).toBe(false)
  })

  it('routes accepted row drops through the folder drop target props', async () => {
    const onEntryDrop = vi.fn()
    render(
      <FileList
        entries={mockEntries}
        dropTargetEntryId="1"
        onEntryDragOver={(entry) => entry.isDir === true}
        onEntryDrop={onEntryDrop}
      />,
    )

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    const fileEntry = screen.getAllByText('file.txt')[0].closest('[role="row"]')
    expect(documentsEntry).toBeTruthy()
    expect(fileEntry).toBeTruthy()

    const dataTransfer = {
      setData: vi.fn(),
      getData: () => '',
      dropEffect: 'none',
      effectAllowed: '',
      types: [],
    }

    expect(fireEvent.dragOver(documentsEntry!, { dataTransfer })).toBe(false)
    expect(documentsEntry?.className).toContain('bg-brand/10')

    expect(fireEvent.drop(documentsEntry!, { dataTransfer })).toBe(false)
    expect(onEntryDrop).toHaveBeenCalledWith(
      expect.objectContaining({ id: '1', name: 'Documents' }),
      expect.anything(),
    )

    expect(fireEvent.dragOver(fileEntry!, { dataTransfer })).toBe(true)
  })

  it('should handle keyboard navigation', async () => {
    const user = userEvent.setup()
    render(<FileList entries={mockEntries} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const listContainer = screen.getByRole('rowgroup')
    listContainer.focus()

    await user.keyboard('{ArrowDown}')

    await waitFor(() => {
      const rows = screen.getAllByRole('row')
      const hasSelection = rows.some((row) =>
        row.className.includes('bg-ui-selected'),
      )
      expect(hasSelection).toBe(true)
    })

    await user.keyboard('{ArrowDown}')

    await waitFor(() => {
      const rows = screen.getAllByRole('row')
      const selectedRows = rows.filter((row) =>
        row.className.includes('bg-ui-selected'),
      )
      expect(selectedRows.length).toBe(1)
    })
  })

  it('should handle select all with Ctrl+A', async () => {
    const user = userEvent.setup()
    render(<FileList entries={mockEntries} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const listContainer = screen.getByRole('rowgroup')
    listContainer.focus()

    await user.keyboard('{Control>}a{/Control}')

    await waitFor(() => {
      const allEntries = screen.getAllByRole('row')
      allEntries.forEach((entry) => {
        expect(entry.className).toContain('bg-ui-selected')
      })
    })
  })

  it('should handle range selection with Shift', async () => {
    const user = userEvent.setup()
    render(<FileList entries={mockEntries} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    await user.click(documentsEntry!)

    await waitFor(() => {
      expect(documentsEntry?.className).toContain('bg-ui-selected')
    })

    const readmeEntry = screen
      .getAllByText('README.md')[0]
      .closest('[role="row"]')
    await user.keyboard('{Shift>}')
    await user.click(readmeEntry!)
    await user.keyboard('{/Shift}')

    await waitFor(() => {
      const selectedRows = screen
        .getAllByRole('row')
        .filter((row) => row.className.includes('bg-ui-selected'))
      expect(selectedRows.length).toBeGreaterThan(1)
    })
  })

  it('should handle toggle selection with Ctrl/Cmd', async () => {
    const user = userEvent.setup()
    render(<FileList entries={mockEntries} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    await user.click(documentsEntry!)

    await waitFor(() => {
      expect(documentsEntry?.className).toContain('bg-ui-selected')
    })

    const picturesEntry = screen
      .getAllByText('Pictures')[0]
      .closest('[role="row"]')
    await user.keyboard('{Control>}')
    await user.click(picturesEntry!)
    await user.keyboard('{/Control}')

    await waitFor(() => {
      const selectedRows = screen
        .getAllByRole('row')
        .filter((row) => row.className.includes('bg-ui-selected'))
      expect(selectedRows.length).toBe(2)
    })
  })

  it('should truncate long filenames with ellipsis', async () => {
    render(<FileList entries={longFilenameEntries} />)

    await waitFor(() => {
      expect(screen.getAllByText('getting-started.md')[0]).toBeTruthy()
    })

    // Find the filename span elements
    const rows = screen.getAllByRole('row')
    expect(rows.length).toBe(3)

    // Check that each row's name container has truncation styling
    for (const row of rows) {
      const nameContainer = row.querySelector('span')
      expect(nameContainer).toBeTruthy()

      // The span should have truncate class for text-overflow: ellipsis
      const hasOverflowHidden =
        nameContainer?.className.includes('truncate') ||
        nameContainer?.className.includes('overflow-hidden')
      expect(
        hasOverflowHidden,
        'Filename span should have truncation styling to prevent text wrapping',
      ).toBe(true)
    }
  })

  it('should call onOpen with directory entry when double-clicking a directory', async () => {
    const user = userEvent.setup()
    const onOpen = vi.fn<(entries: FileEntry[]) => void>()
    render(<FileList entries={mockEntries} onOpen={onOpen} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    // Double-click the Documents directory
    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    await user.dblClick(documentsEntry!)

    await waitFor(
      () => {
        expect(onOpen).toHaveBeenCalledOnce()
        const entries = onOpen.mock.calls[0][0]
        expect(entries).toHaveLength(1)
        expect(entries[0].name).toBe('Documents')
        expect(entries[0].isDir).toBe(true)
      },
      { timeout: 1000 },
    )
  })

  it('should call onOpen with file entry when double-clicking a file', async () => {
    const user = userEvent.setup()
    const onOpen = vi.fn<(entries: FileEntry[]) => void>()
    render(<FileList entries={mockEntries} onOpen={onOpen} />)

    await waitFor(() => {
      expect(screen.getAllByText('file.txt')[0]).toBeTruthy()
    })

    // Double-click the file.txt file
    const fileEntry = screen.getAllByText('file.txt')[0].closest('[role="row"]')
    await user.dblClick(fileEntry!)

    await waitFor(
      () => {
        expect(onOpen).toHaveBeenCalledOnce()
        const entries = onOpen.mock.calls[0][0]
        expect(entries).toHaveLength(1)
        expect(entries[0].name).toBe('file.txt')
        expect(entries[0].isDir).toBe(false)
      },
      { timeout: 1000 },
    )
  })

  it('should call onOpen with all selected files when double-clicking with multiple selection', async () => {
    const user = userEvent.setup()
    const onOpen = vi.fn<(entries: FileEntry[]) => void>()
    render(<FileList entries={mockEntries} onOpen={onOpen} />)

    await waitFor(() => {
      expect(screen.getAllByText('file.txt')[0]).toBeTruthy()
    })

    // First click on file.txt to select it
    const fileEntry = screen.getAllByText('file.txt')[0].closest('[role="row"]')
    await user.click(fileEntry!)

    await waitFor(() => {
      expect(fileEntry?.className).toContain('bg-ui-selected')
    })

    // Ctrl+click README.md to add to selection
    const readmeEntry = screen
      .getAllByText('README.md')[0]
      .closest('[role="row"]')
    await user.keyboard('{Control>}')
    await user.click(readmeEntry!)
    await user.keyboard('{/Control}')

    await waitFor(() => {
      const selectedRows = screen
        .getAllByRole('row')
        .filter((row) => row.className.includes('bg-ui-selected'))
      expect(selectedRows.length).toBe(2)
    })

    // Double-click on one of the selected files
    await user.dblClick(readmeEntry!)

    await waitFor(
      () => {
        expect(onOpen).toHaveBeenCalledOnce()
        const entries = onOpen.mock.calls[0][0]
        expect(entries).toHaveLength(2)
        // Should include both selected files
        const names = entries.map((e: FileEntry) => e.name).sort()
        expect(names).toEqual(['README.md', 'file.txt'])
      },
      { timeout: 1000 },
    )
  })

  it('should call onOpen with mixed selection when double-clicking with files and directories selected', async () => {
    const user = userEvent.setup()
    const onOpen = vi.fn<(entries: FileEntry[]) => void>()
    render(<FileList entries={mockEntries} onOpen={onOpen} />)

    await waitFor(() => {
      expect(screen.getAllByText('Documents')[0]).toBeTruthy()
    })

    // Click on Documents directory to select it
    const documentsEntry = screen
      .getAllByText('Documents')[0]
      .closest('[role="row"]')
    await user.click(documentsEntry!)

    await waitFor(() => {
      expect(documentsEntry?.className).toContain('bg-ui-selected')
    })

    // Ctrl+click file.txt to add to selection
    const fileEntry = screen.getAllByText('file.txt')[0].closest('[role="row"]')
    await user.keyboard('{Control>}')
    await user.click(fileEntry!)
    await user.keyboard('{/Control}')

    await waitFor(() => {
      const selectedRows = screen
        .getAllByRole('row')
        .filter((row) => row.className.includes('bg-ui-selected'))
      expect(selectedRows.length).toBe(2)
    })

    // Double-click on the file
    await user.dblClick(fileEntry!)

    await waitFor(
      () => {
        expect(onOpen).toHaveBeenCalledOnce()
        const entries = onOpen.mock.calls[0][0]
        expect(entries).toHaveLength(2)
        // Should include the directory and file
        const dirEntry = entries.find((e: FileEntry) => e.isDir)
        const fileEntryResult = entries.find((e: FileEntry) => !e.isDir)
        expect(dirEntry?.name).toBe('Documents')
        expect(fileEntryResult?.name).toBe('file.txt')
      },
      { timeout: 1000 },
    )
  })
})

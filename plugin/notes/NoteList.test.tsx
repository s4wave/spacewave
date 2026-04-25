import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import type { NotebookSource } from './proto/notebook.pb.js'

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: vi.fn(() => ({
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  })),
  useUnixFSHandle: vi.fn(() => ({
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  })),
  useUnixFSHandleEntries: vi.fn(() => ({
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  })),
}))

vi.mock('@s4wave/sdk/space/object-uri.js', () => ({
  parseObjectUri: vi.fn((ref: string) => {
    const parts = ref.split('/-/')
    return { objectKey: parts[0] ?? '', path: parts[1] ?? '' }
  }),
}))

vi.mock('@s4wave/sdk/unixfs/index.js', () => ({
  MknodType: { FILE: 1, DIRECTORY: 2 },
}))

import NoteList from './NoteList.js'
import {
  useUnixFSHandle,
  useUnixFSHandleEntries,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

const mockWorldState = {
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
}

function makeDirectoryHandle(files: Record<string, string> = {}) {
  return {
    lookup: vi.fn(async (name: string) => ({
      readAt: vi.fn(async () => ({
        data: new TextEncoder().encode(files[name] ?? ''),
        eof: true,
      })),
      writeAt: vi.fn(async () => {}),
      release: vi.fn(),
    })),
    mknod: vi.fn(async () => {}),
    mkdirAll: vi.fn(async () => {}),
    remove: vi.fn(async () => {}),
    rename: vi.fn(async () => {}),
  }
}

function mockDirectory(
  entries: Array<{ name: string; isDir: boolean }>,
  files: Record<string, string> = {},
) {
  const handle = makeDirectoryHandle(files)
  vi.mocked(useUnixFSHandle).mockReturnValue({
    value: handle,
    loading: false,
    error: null,
    retry: vi.fn(),
  } as never)
  vi.mocked(useUnixFSHandleEntries).mockReturnValue({
    value: entries,
    loading: false,
    error: null,
    retry: vi.fn(),
  } as never)
  return handle
}

function renderList(
  props: Partial<React.ComponentProps<typeof NoteList>> = {},
) {
  const source =
    'source' in props
      ? props.source
      : ({
          name: 'Docs',
          ref: 'obj-key/-/docs',
        } satisfies NotebookSource)

  return render(
    <NoteList
      source={source}
      worldState={mockWorldState as never}
      selectedNote={props.selectedNote ?? ''}
      currentPath={props.currentPath}
      onSelectNote={props.onSelectNote ?? vi.fn()}
      onChangePath={props.onChangePath}
      onNoteRenamed={props.onNoteRenamed}
      onNoteDeleted={props.onNoteDeleted}
      filterTag={props.filterTag}
      filterStatus={props.filterStatus}
      onFilterTagChange={props.onFilterTagChange}
      onFilterStatusChange={props.onFilterStatusChange}
      onCreateNote={props.onCreateNote}
      renderEntryExtra={props.renderEntryExtra}
    />,
  )
}

describe('NoteList', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('shows "Select a source" when source is undefined', () => {
    renderList({ source: undefined })
    expect(screen.getByText('Select a source')).toBeDefined()
  })

  it('shows "Invalid source ref" when source has no ref', () => {
    renderList({ source: { name: 'Empty' } })
    expect(screen.getByText('Invalid source ref')).toBeDefined()
  })

  it('shows loading state when entries are loading', () => {
    vi.mocked(useUnixFSHandleEntries).mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    } as never)

    renderList()
    expect(screen.getByText('Loading...')).toBeDefined()
  })

  it('shows error message when entries have an error', () => {
    vi.mocked(useUnixFSHandleEntries).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Network failure'),
      retry: vi.fn(),
    } as never)

    renderList()
    expect(screen.getByText('Network failure')).toBeDefined()
  })

  it('shows empty state when the directory has no notes or folders', () => {
    mockDirectory([])

    renderList()
    expect(screen.getByText('No notes yet')).toBeDefined()
    expect(screen.getByText('Create your first note')).toBeDefined()
  })

  it('renders markdown file entries and ignores non-markdown files', async () => {
    mockDirectory(
      [
        { name: 'hello.md', isDir: false },
        { name: 'world.md', isDir: false },
        { name: 'image.png', isDir: false },
      ],
      {
        'hello.md': '# Hello',
        'world.md': '# World',
      },
    )

    renderList()
    await waitFor(() => {
      expect(screen.getByText('hello')).toBeDefined()
      expect(screen.getByText('world')).toBeDefined()
      expect(screen.queryByText('image')).toBeNull()
    })
  })

  it('renders undated markdown notes that stay notebook-visible', async () => {
    mockDirectory(
      [{ name: 'work-note.md', isDir: false }],
      {
        'work-note.md':
          '---\nstatus: in-progress\ntags: [internal]\n---\n\n# Work Note',
      },
    )

    renderList()
    await waitFor(() => {
      expect(screen.getByText('work-note')).toBeDefined()
    })
  })

  it('calls onSelectNote with the full path when a note is clicked', async () => {
    mockDirectory([{ name: 'note.md', isDir: false }], { 'note.md': '# note' })
    const onSelectNote = vi.fn()

    renderList({ currentPath: 'nested', onSelectNote })

    await waitFor(() => {
      fireEvent.click(screen.getByText('note'))
      expect(onSelectNote).toHaveBeenCalledWith('nested/note.md')
    })
  })

  it('highlights the selected note in the current directory', async () => {
    mockDirectory(
      [
        { name: 'a.md', isDir: false },
        { name: 'b.md', isDir: false },
      ],
      {
        'a.md': '# a',
        'b.md': '# b',
      },
    )

    renderList({ selectedNote: 'nested/b.md', currentPath: 'nested' })

    await waitFor(() => {
      const row = screen.getByText('b').closest('div')
      expect(row?.className).toContain('bg-list-active-selection-background')
    })
  })

  it('renders directories and navigates into them', () => {
    mockDirectory([{ name: 'projects', isDir: true }])
    const onChangePath = vi.fn()

    renderList({ onChangePath })

    fireEvent.click(screen.getByText('projects'))
    expect(onChangePath).toHaveBeenCalledWith('projects')
  })

  it('shows the current folder path and supports navigating up', () => {
    mockDirectory([])
    const onChangePath = vi.fn()

    renderList({ currentPath: 'projects/client-a', onChangePath })

    expect(screen.getByText('/projects/client-a')).toBeDefined()
    fireEvent.click(screen.getByTitle('Up one level'))
    expect(onChangePath).toHaveBeenCalledWith('projects')
  })

  it('filters directories and notes by search query', async () => {
    mockDirectory(
      [
        { name: 'projects', isDir: true },
        { name: 'alpha.md', isDir: false },
        { name: 'beta.md', isDir: false },
      ],
      {
        'alpha.md': '# alpha',
        'beta.md': '# beta',
      },
    )

    renderList()

    const input = await screen.findByPlaceholderText('Search notes...')
    fireEvent.change(input, { target: { value: 'proj' } })
    expect(screen.getByText('projects')).toBeDefined()
    expect(screen.queryByText('alpha')).toBeNull()

    fireEvent.change(input, { target: { value: 'alph' } })
    await waitFor(() => {
      expect(screen.getByText('alpha')).toBeDefined()
      expect(screen.queryByText('projects')).toBeNull()
      expect(screen.queryByText('beta')).toBeNull()
    })
  })

  it('filters notes by frontmatter tag and clears the tag filter', async () => {
    mockDirectory(
      [
        { name: 'alpha.md', isDir: false },
        { name: 'beta.md', isDir: false },
      ],
      {
        'alpha.md': '---\ntags: [focus]\n---\n\n# Alpha',
        'beta.md': '---\ntags: [other]\n---\n\n# Beta',
      },
    )
    const onFilterTagChange = vi.fn()

    renderList({ filterTag: 'focus', onFilterTagChange })

    await waitFor(() => {
      expect(screen.getByText('alpha')).toBeDefined()
      expect(screen.queryByText('beta')).toBeNull()
    })
    fireEvent.click(screen.getByTitle('Clear tag filter'))
    expect(onFilterTagChange).toHaveBeenCalledWith(undefined)
  })

  it('filters notes by frontmatter status and clears the status filter', async () => {
    mockDirectory(
      [
        { name: 'todo.md', isDir: false },
        { name: 'done.md', isDir: false },
      ],
      {
        'todo.md': '---\nstatus: todo\n---\n\n# Todo',
        'done.md': '---\nstatus: done\n---\n\n# Done',
      },
    )
    const onFilterStatusChange = vi.fn()

    renderList({ filterStatus: 'done', onFilterStatusChange })

    await waitFor(() => {
      expect(screen.getAllByText('done').length).toBe(2)
      expect(screen.queryByText('todo')).toBeNull()
    })
    fireEvent.click(screen.getByTitle('Clear status filter'))
    expect(onFilterStatusChange).toHaveBeenCalledWith(undefined)
  })

  it('creates a folder in the current directory', () => {
    const handle = mockDirectory([])
    vi.stubGlobal('prompt', vi.fn(() => 'projects/client-a'))

    renderList()

    fireEvent.click(screen.getByTitle('New folder'))
    expect(handle.mkdirAll).toHaveBeenCalledWith(['projects', 'client-a'])
  })

  it('renames a note and reports the path change', async () => {
    const handle = mockDirectory(
      [{ name: 'draft.md', isDir: false }],
      { 'draft.md': '# Draft' },
    )
    vi.stubGlobal('prompt', vi.fn(() => 'final'))
    const onNoteRenamed = vi.fn()

    renderList({ currentPath: 'projects', onNoteRenamed })

    await waitFor(() => {
      fireEvent.click(screen.getByTitle('Rename note'))
      expect(handle.rename).toHaveBeenCalledWith('draft.md', 'final.md')
      expect(onNoteRenamed).toHaveBeenCalledWith(
        'projects/draft.md',
        'projects/final.md',
      )
    })
  })

  it('deletes a note and reports the deleted path', async () => {
    const handle = mockDirectory(
      [{ name: 'draft.md', isDir: false }],
      { 'draft.md': '# Draft' },
    )
    vi.stubGlobal('confirm', vi.fn(() => true))
    const onNoteDeleted = vi.fn()

    renderList({ currentPath: 'projects', onNoteDeleted })

    await waitFor(() => {
      fireEvent.click(screen.getByTitle('Delete note'))
      expect(handle.remove).toHaveBeenCalledWith(['draft.md'])
      expect(onNoteDeleted).toHaveBeenCalledWith('projects/draft.md')
    })
  })
})

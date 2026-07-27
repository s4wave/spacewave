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
    lookup: vi.fn((name: string) =>
      Promise.resolve({
        getSize: vi.fn(() =>
          Promise.resolve(
            BigInt(new TextEncoder().encode(files[name] ?? '').length),
          ),
        ),
        readAt: vi.fn(() =>
          Promise.resolve({
            data: new TextEncoder().encode(files[name] ?? ''),
            eof: true,
          }),
        ),
        writeAt: vi.fn(() => Promise.resolve()),
        release: vi.fn(),
      }),
    ),
    mknod: vi.fn(() => Promise.resolve()),
    uploadFile: vi.fn(() => Promise.resolve(0n)),
    mkdirAll: vi.fn(() => Promise.resolve()),
    remove: vi.fn(() => Promise.resolve()),
    rename: vi.fn(() => Promise.resolve()),
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
      allowedFormats={props.allowedFormats}
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
    })

    renderList()
    expect(screen.getByText('Loading…')).toBeDefined()
  })

  it('shows error message when entries have an error', () => {
    vi.mocked(useUnixFSHandleEntries).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Network failure'),
      retry: vi.fn(),
    })

    renderList()
    expect(screen.getByText('Network failure')).toBeDefined()
  })

  it('shows empty state when the directory has no notes or folders', () => {
    mockDirectory([])

    renderList()
    expect(screen.getByText('No notes yet')).toBeDefined()
    expect(screen.getByText('Create your first note')).toBeDefined()
  })

  it('renders Markdown and Org file entries while ignoring other files', async () => {
    mockDirectory(
      [
        { name: 'hello.md', isDir: false },
        { name: 'world.org', isDir: false },
        { name: 'image.png', isDir: false },
      ],
      {
        'hello.md': '# Hello',
        'world.org': '#+TITLE: World\n\n* World',
      },
    )

    renderList()
    await waitFor(() => {
      expect(screen.getByText('hello')).toBeDefined()
      expect(screen.getByText('World')).toBeDefined()
      expect(screen.queryByText('image')).toBeNull()
    })
  })

  it('renders undated markdown notes that stay notebook-visible', async () => {
    mockDirectory([{ name: 'work-note.md', isDir: false }], {
      'work-note.md':
        '---\nstatus: in-progress\ntags: [internal]\n---\n\n# Work Note',
    })

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

    const input = await screen.findByPlaceholderText('Search notes…')
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

    renderList()

    fireEvent.click(screen.getByTitle('New folder'))
    fireEvent.change(screen.getByLabelText('Folder name'), {
      target: { value: 'projects/client-a' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create folder' }))

    expect(handle.mkdirAll).toHaveBeenCalledWith(['projects', 'client-a'])
  })

  it('creates a note with template content atomically', async () => {
    const handle = mockDirectory([])
    const onSelectNote = vi.fn()

    renderList({ currentPath: 'projects', onSelectNote })

    fireEvent.click(screen.getByText('Create your first note'))

    await waitFor(() => {
      expect(handle.uploadFile).toHaveBeenCalledWith(
        'untitled.md',
        expect.anything(),
        expect.anything(),
      )
      expect(onSelectNote).toHaveBeenCalledWith('projects/untitled.md')
    })

    expect(handle.mknod).not.toHaveBeenCalled()
    const uploadCall = handle.uploadFile.mock.calls[0] as unknown as [
      string,
      bigint,
      ReadableStream<Uint8Array>,
    ]
    const text = new TextDecoder().decode(await readStreamBytes(uploadCall[2]))
    expect(text).toContain('# untitled')
  })

  it('creates an Org note with template content atomically', async () => {
    const handle = mockDirectory([])
    const onSelectNote = vi.fn()

    renderList({ currentPath: 'projects', onSelectNote })

    fireEvent.click(screen.getByTitle('New Org note'))

    await waitFor(() => {
      expect(handle.uploadFile).toHaveBeenCalledWith(
        'untitled.org',
        expect.anything(),
        expect.anything(),
      )
      expect(onSelectNote).toHaveBeenCalledWith('projects/untitled.org')
    })

    const uploadCall = handle.uploadFile.mock.calls[0] as unknown as [
      string,
      bigint,
      ReadableStream<Uint8Array>,
    ]
    const text = new TextDecoder().decode(await readStreamBytes(uploadCall[2]))
    expect(text).toBe('#+TITLE: untitled\n\n* untitled\n\n')
  })

  it('renames a note and reports the path change', async () => {
    const handle = mockDirectory([{ name: 'draft.md', isDir: false }], {
      'draft.md': '# Draft',
    })
    const onNoteRenamed = vi.fn()

    renderList({ currentPath: 'projects', onNoteRenamed })

    await waitFor(() => {
      fireEvent.click(screen.getByTitle('Rename note'))
    })
    fireEvent.change(screen.getByLabelText('Note name'), {
      target: { value: 'final' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => {
      expect(handle.rename).toHaveBeenCalledWith('draft.md', 'final.md')
      expect(onNoteRenamed).toHaveBeenCalledWith(
        'projects/draft.md',
        'projects/final.md',
      )
    })
  })

  it('renames an Org note without changing its extension', async () => {
    const handle = mockDirectory([{ name: 'draft.org', isDir: false }], {
      'draft.org': '#+TITLE: Draft\n\n* Draft',
    })
    const onNoteRenamed = vi.fn()

    renderList({ currentPath: 'projects', onNoteRenamed })

    await waitFor(() => {
      fireEvent.click(screen.getByTitle('Rename note'))
    })
    fireEvent.change(screen.getByLabelText('Note name'), {
      target: { value: 'final' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() => {
      expect(handle.rename).toHaveBeenCalledWith('draft.org', 'final.org')
      expect(onNoteRenamed).toHaveBeenCalledWith(
        'projects/draft.org',
        'projects/final.org',
      )
    })
  })

  it('deletes a note and reports the deleted path', async () => {
    const handle = mockDirectory([{ name: 'draft.md', isDir: false }], {
      'draft.md': '# Draft',
    })
    const onNoteDeleted = vi.fn()

    renderList({ currentPath: 'projects', onNoteDeleted })

    await waitFor(() => {
      fireEvent.click(screen.getByTitle('Delete note'))
    })
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(handle.remove).toHaveBeenCalledWith(['draft.md'])
      expect(onNoteDeleted).toHaveBeenCalledWith('projects/draft.md')
    })
  })
})

async function readStreamBytes(stream: ReadableStream<Uint8Array>) {
  return new Uint8Array(await new Response(stream).arrayBuffer())
}

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  fireEvent,
  render,
  screen,
  cleanup,
  waitFor,
} from '@testing-library/react'
import { StateNamespaceProvider, atom } from '@s4wave/web/state/index.js'

const mockUseWorldObjectMessageState = vi.hoisted(() => vi.fn())

vi.mock('./useWorldObjectMessageState.js', () => ({
  useWorldObjectMessageState: mockUseWorldObjectMessageState,
}))

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: vi.fn(() => ({
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  })),
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: vi.fn(() => 'mock-object-key'),
}))

// The UnixFS hooks return the same resource on every render, as the real
// hooks do while nothing changes. A fresh object per render would change the
// note list's memoized entries each render and restart its note read without
// end.
const unixfs = vi.hoisted(() => {
  const empty = (value: unknown) => ({
    value,
    loading: false,
    error: null,
    retry: () => {},
  })
  return { handle: empty(null), entries: empty([]), text: empty(null) }
})

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: vi.fn(() => unixfs.handle),
  useUnixFSHandle: vi.fn(() => unixfs.handle),
  useUnixFSHandleEntries: vi.fn(() => unixfs.entries),
  useUnixFSHandleTextContent: vi.fn(() => unixfs.text),
}))

vi.mock('@s4wave/sdk/space/object-uri.js', () => ({
  parseObjectUri: vi.fn((ref: string) => {
    const parts = ref.split('/-/')
    return { objectKey: parts[0] ?? '', path: parts[1] ?? '' }
  }),
}))

import NotebookViewer from './NotebookViewer.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { NotebookSource } from './proto/notebook.pb.js'
import { NotebookEditOutcome } from './sdk/notebook.pb.js'

const mockObjectInfo = { key: 'mock-key', typeId: 'notes/notebook' }
const mockWorldState = {
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
}

function renderViewer() {
  const rootAtom = atom<Record<string, unknown>>({})
  const tree = () => (
    <StateNamespaceProvider rootAtom={rootAtom} namespace={['viewer-test']}>
      <NotebookViewer
        objectInfo={mockObjectInfo as never}
        worldState={mockWorldState as never}
      />
    </StateNamespaceProvider>
  )
  const result = render(tree())
  return { ...result, rerender: () => result.rerender(tree()) }
}

// watchSources makes the mocked watch show sources, as the World watch does
// after a commit.
function watchSources(sources: NotebookSource[]) {
  mockUseWorldObjectMessageState.mockReturnValue({
    state: {
      value: { sources },
      loading: false,
      error: null,
      retry: vi.fn(),
    },
    loading: false,
    sources,
  })
}

// fakeNotebook is a Notebook handle over a stored list that applies the
// edit op's guards: a Remove applies only while its index still holds the
// source the caller saw, and a Reorder only while the list is unchanged.
function fakeNotebook(stored: NotebookSource[]) {
  const same = (a?: NotebookSource, b?: NotebookSource) =>
    !!a && !!b && a.name === b.name && a.ref === b.ref
  const handle = {
    stored,
    addSource: vi.fn(async (source: NotebookSource) => {
      handle.stored = [...handle.stored, source]
    }),
    removeSource: vi.fn(
      async (
        index: number,
        source: NotebookSource,
      ): Promise<NotebookEditOutcome> => {
        if (!same(handle.stored[index], source))
          return NotebookEditOutcome.CHANGED
        handle.stored = handle.stored.filter((_, i) => i !== index)
        return NotebookEditOutcome.APPLIED
      },
    ),
    reorderSources: vi.fn(
      async (
        order: number[],
        seen: NotebookSource[],
      ): Promise<NotebookEditOutcome> => {
        const unchanged =
          seen.length === handle.stored.length &&
          seen.every((source, i) => same(source, handle.stored[i]))
        if (!unchanged) return NotebookEditOutcome.CHANGED
        handle.stored = order.map((i) => handle.stored[i])
        return NotebookEditOutcome.APPLIED
      },
    ),
  }
  vi.mocked(useAccessTypedHandle).mockReturnValue({
    value: handle,
    loading: false,
    error: null,
    retry: vi.fn(),
  } as never)
  return handle
}

const A = { name: 'A', ref: 'key/-/a' }
const B = { name: 'B', ref: 'key/-/b' }
const C = { name: 'C', ref: 'key/-/c' }

describe('NotebookViewer', () => {
  beforeEach(() => {
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      },
      sources: [],
    })
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders loading state when notebook state is loading', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })
    renderViewer()
    expect(screen.getByText('Loading the notebook')).toBeDefined()
  })

  it('renders content while notebook mutation handle is loading', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: {
          sources: [{ name: 'Docs', ref: 'key/-/docs' }],
        },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      sources: [{ name: 'Docs', ref: 'key/-/docs' }],
    })

    renderViewer()
    expect(screen.getAllByText('Docs')).toHaveLength(2)
  })

  it('renders error state when notebook state has error', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Connection refused'),
      last: null,
      retry: vi.fn(),
    })
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: null,
        loading: false,
        error: new Error('Connection refused'),
        retry: vi.fn(),
      },
      sources: [],
    })

    renderViewer()
    expect(
      screen.getByText('Could not read the notebook: Connection refused'),
    ).toBeDefined()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDefined()
  })

  it('renders empty sources message when notebook has no sources', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: { sources: [] },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      sources: [],
    })

    renderViewer()
    expect(
      screen.getByText('No sources configured for this notebook'),
    ).toBeDefined()
    expect(screen.getByRole('button', { name: 'Add source' })).toBeDefined()
  })

  it('adds a source from the in-app dialog', async () => {
    const addSource = vi.fn(() => Promise.resolve())
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: { addSource },
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: { sources: [] },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      sources: [],
    })

    renderViewer()
    fireEvent.click(screen.getByRole('button', { name: 'Add source' }))
    fireEvent.change(screen.getByLabelText('Source name'), {
      target: { value: 'Docs' },
    })
    fireEvent.change(screen.getByLabelText('Source ref'), {
      target: { value: 'key/-/docs' },
    })
    const addButtons = screen.getAllByRole('button', { name: 'Add source' })
    fireEvent.click(addButtons[addButtons.length - 1])

    await waitFor(() => {
      expect(addSource).toHaveBeenCalledWith({
        name: 'Docs',
        ref: 'key/-/docs',
      })
    })
  })

  it('renders "Select a note to view" when sources exist but no note selected', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: {
          sources: [{ name: 'Docs', ref: 'key/-/docs' }],
        },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      sources: [{ name: 'Docs', ref: 'key/-/docs' }],
    })

    renderViewer()
    expect(screen.getByText('Select a note to view')).toBeDefined()
  })

  it('names the mobile notebook navigation for its current action', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: {
          sources: [{ name: 'My Docs', ref: 'key/-/docs' }],
        },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      sources: [{ name: 'My Docs', ref: 'key/-/docs' }],
    })

    renderViewer()
    const openButton = screen.getByRole('button', {
      name: 'Open notebook navigation',
    })
    fireEvent.click(openButton)
    const closeButton = screen.getByRole('button', {
      name: 'Close notebook navigation',
    })
    fireEvent.click(closeButton)
    expect(
      screen.getByRole('button', { name: 'Open notebook navigation' }),
    ).toBeDefined()
  })

  it('renders three-panel layout when loaded with sources', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: {
          sources: [{ name: 'My Docs', ref: 'key/-/docs' }],
        },
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      sources: [{ name: 'My Docs', ref: 'key/-/docs' }],
    })

    const { container } = renderViewer()
    // The outer container should have the three-panel flex layout.
    const outer = container.firstElementChild as HTMLElement
    expect(outer.className).toContain('flex')
    // Should render the sidebar with source name.
    expect(screen.getAllByText('My Docs')).toHaveLength(2)
  })

  it('removes only the source the person clicked when the list changes under the dialog', async () => {
    const notebook = fakeNotebook([A, B, C])
    watchSources([A, B, C])
    const view = renderViewer()

    // The person clicks Remove on B, at index 1.
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove source' })[1])
    expect(screen.getByText('Remove “B” from the notebook?')).toBeDefined()

    // Another writer removes A while the dialog is open.
    notebook.stored = [B, C]
    watchSources([B, C])
    view.rerender()
    expect(screen.getByText('Remove “B” from the notebook?')).toBeDefined()

    // The confirm sends B, which index 1 no longer holds, so nothing goes.
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    await waitFor(() => {
      expect(
        screen.getByText(
          'The sources changed before this edit, so it was not applied.',
        ),
      ).toBeDefined()
    })
    expect(notebook.removeSource).toHaveBeenCalledWith(1, B)
    await expect(notebook.removeSource.mock.results[0].value).resolves.toBe(
      NotebookEditOutcome.CHANGED,
    )
    expect(notebook.stored).toEqual([B, C])
    expect(screen.queryByText('Remove “B” from the notebook?')).toBeNull()
  })

  it('disables the Remove confirm while it sends, and offers it again after a failure', async () => {
    const notebook = fakeNotebook([A, B])
    watchSources([A, B])
    let fail!: (err: Error) => void
    notebook.removeSource.mockImplementationOnce(
      () => new Promise((_, reject) => (fail = reject)),
    )
    renderViewer()

    fireEvent.click(screen.getAllByRole('button', { name: 'Remove source' })[1])
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    await waitFor(() => {
      expect(
        (screen.getByRole('button', { name: 'Remove' }) as HTMLButtonElement)
          .disabled,
      ).toBe(true)
    })

    fail(new Error('stream closed'))
    expect(
      await screen.findByText('Could not remove “B”: stream closed'),
    ).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    await waitFor(() => {
      expect(notebook.stored).toEqual([A])
    })
    expect(notebook.removeSource).toHaveBeenNthCalledWith(2, 1, B)
    await waitFor(() => {
      expect(screen.queryByText('Remove “B” from the notebook?')).toBeNull()
    })
  })

  it('keeps the typed source after a failed Add and sends it again', async () => {
    const notebook = fakeNotebook([])
    watchSources([])
    notebook.addSource.mockRejectedValueOnce(new Error('notebook not found'))
    renderViewer()

    fireEvent.click(screen.getByRole('button', { name: 'Add source' }))
    fireEvent.change(screen.getByLabelText('Source name'), {
      target: { value: 'Docs' },
    })
    fireEvent.change(screen.getByLabelText('Source ref'), {
      target: { value: 'key/-/docs' },
    })
    const addButtons = screen.getAllByRole('button', { name: 'Add source' })
    fireEvent.click(addButtons[addButtons.length - 1])

    expect(
      await screen.findByText('Could not add the source: notebook not found'),
    ).toBeDefined()
    expect(
      (screen.getByLabelText('Source ref') as HTMLInputElement).value,
    ).toBe('key/-/docs')
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    await waitFor(() => {
      expect(notebook.stored).toEqual([{ name: 'Docs', ref: 'key/-/docs' }])
    })
    expect(notebook.addSource).toHaveBeenCalledTimes(2)
    await waitFor(() => {
      expect(screen.queryByLabelText('Source ref')).toBeNull()
    })
  })

  it('shows a failed Reorder by the list and resends the same order', async () => {
    const notebook = fakeNotebook([A, B])
    watchSources([A, B])
    notebook.reorderSources.mockRejectedValueOnce(new Error('stream closed'))
    renderViewer()

    fireEvent.click(
      screen.getAllByRole('button', { name: 'Move source down' })[0],
    )
    expect(
      await screen.findByText('Could not move the source: stream closed'),
    ).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    await waitFor(() => {
      expect(notebook.stored).toEqual([B, A])
    })
    expect(notebook.reorderSources).toHaveBeenNthCalledWith(2, [1, 0], [A, B])
    expect(
      screen.queryByText('Could not move the source: stream closed'),
    ).toBeNull()
  })
  it('sends no Add while a Reorder is held, and shows the Reorder failure by the list', async () => {
    const notebook = fakeNotebook([A, B])
    watchSources([A, B])
    let fail!: (err: Error) => void
    notebook.reorderSources.mockImplementationOnce(
      () => new Promise((_, reject) => (fail = reject)),
    )
    renderViewer()

    // The person opens Add and types, then moves a source behind the dialog.
    fireEvent.click(screen.getByRole('button', { name: 'Add source' }))
    fireEvent.change(screen.getByLabelText('Source name'), {
      target: { value: 'Docs' },
    })
    fireEvent.change(screen.getByLabelText('Source ref'), {
      target: { value: 'key/-/docs' },
    })
    fireEvent.click(
      screen.getAllByRole('button', {
        name: 'Move source down',
        hidden: true,
      })[0],
    )

    // While the Reorder sends, every control that starts an edit is off.
    const addButtons = () =>
      screen.getAllByRole('button', {
        name: 'Add source',
        hidden: true,
      }) as HTMLButtonElement[]
    await waitFor(() => {
      expect(addButtons().every((button) => button.disabled)).toBe(true)
    })
    fireEvent.click(addButtons()[addButtons().length - 1])
    expect(notebook.addSource).not.toHaveBeenCalled()

    // The Reorder's failure shows by the list, never in the Add dialog.
    fail(new Error('stream closed'))
    expect(
      await screen.findByText('Could not move the source: stream closed'),
    ).toBeDefined()
    expect(screen.queryByText(/Could not add the source/)).toBeNull()
    expect(notebook.addSource).not.toHaveBeenCalled()
    expect(notebook.stored).toEqual([A, B])
  })

  it('keeps the Remove dialog open on Escape while it sends, to show the result', async () => {
    const notebook = fakeNotebook([A, B])
    watchSources([A, B])
    let fail!: (err: Error) => void
    notebook.removeSource.mockImplementationOnce(
      () => new Promise((_, reject) => (fail = reject)),
    )
    renderViewer()

    fireEvent.click(screen.getAllByRole('button', { name: 'Remove source' })[1])
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    await waitFor(() => {
      expect(
        (screen.getByRole('button', { name: 'Cancel' }) as HTMLButtonElement)
          .disabled,
      ).toBe(true)
    })

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(screen.getByText('Remove “B” from the notebook?')).toBeDefined()

    fail(new Error('stream closed'))
    expect(
      await screen.findByText('Could not remove “B”: stream closed'),
    ).toBeDefined()
  })

  it('selects the added source by identity when another writer added one too', async () => {
    const notebook = fakeNotebook([A])
    watchSources([A])
    const view = renderViewer()

    fireEvent.click(screen.getByRole('button', { name: 'Add source' }))
    fireEvent.change(screen.getByLabelText('Source name'), {
      target: { value: 'C' },
    })
    fireEvent.change(screen.getByLabelText('Source ref'), {
      target: { value: C.ref },
    })
    const addButtons = screen.getAllByRole('button', { name: 'Add source' })
    fireEvent.click(addButtons[addButtons.length - 1])
    await waitFor(() => {
      expect(notebook.stored).toEqual([A, C])
    })

    // Another writer's B lands before ours in the watched list.
    watchSources([A, B, C])
    view.rerender()
    // The bar names the selected source beside its sidebar entry.
    await waitFor(() => {
      expect(screen.getAllByText('C')).toHaveLength(2)
    })
    expect(screen.getAllByText('B')).toHaveLength(1)
  })
})

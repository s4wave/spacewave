import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  render,
  screen,
  cleanup,
  fireEvent,
  waitFor,
} from '@testing-library/react'

const mockEditorRender = vi.hoisted(() => vi.fn())

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
  useUnixFSHandleTextContent: vi.fn(() => ({
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

vi.mock('./LexicalEditor.js', () => ({
  default: ({
    content,
    format,
    onSave,
    composerKey,
    onDraftChange,
  }: {
    content: string
    format: string
    onSave: (content: string) => Promise<void>
    composerKey?: string
    onDraftChange?: (content: string) => void
  }) => {
    mockEditorRender()
    return (
      <div
        data-testid="lexical-editor"
        data-content={content}
        data-format={format}
        data-composer-key={composerKey}
      >
        <button
          type="button"
          onClick={() => void onSave('saved-content').catch(() => {})}
        >
          mock-save
        </button>
        <button
          type="button"
          onClick={() => void onSave(content).catch(() => {})}
        >
          mock-save-current
        </button>
        <button
          type="button"
          onClick={() => void onSave('newer-content').catch(() => {})}
        >
          mock-save-new
        </button>
        <button type="button" onClick={() => onDraftChange?.('unsent Y')}>
          mock-draft-new
        </button>
      </div>
    )
  },
}))

vi.mock('./FrontmatterDisplay.js', () => ({
  default: ({ frontmatter }: { frontmatter: Record<string, unknown> }) => (
    <div data-testid="frontmatter-display">
      {frontmatter.tags
        ? `tags: ${(frontmatter.tags as string[]).join(',')}`
        : null}
    </div>
  ),
}))

import NoteContentView from './NoteContentView.js'
import * as org from './org/org.js'
import {
  useUnixFSHandle,
  useUnixFSHandleTextContent,
} from '@s4wave/web/hooks/useUnixFSHandle.js'

const mockWorldState = {
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
}

function mockWritableHandle() {
  vi.mocked(useUnixFSHandle).mockReturnValue({
    value: {
      writeAt: vi.fn(() => Promise.resolve(0n)),
      truncate: vi.fn(() => Promise.resolve()),
    } as never,
    loading: false,
    error: null,
    retry: vi.fn(),
  })
}

describe('NoteContentView', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    vi.restoreAllMocks()
    mockEditorRender.mockReset()
  })

  it.each([
    ['metadata', 'path', 'first/-/docs', 'other.org'],
    ['editor', 'path', 'first/-/docs', 'other.org'],
    ['metadata', 'source', 'second/-/docs', 'note.org'],
    ['editor', 'source', 'second/-/docs', 'note.org'],
  ])(
    'contains a %s parse failure and recovers after changing %s',
    (failurePoint, _selection, nextSource, nextNote) => {
      // Fail either metadata parsing or editor initialization for the selected file.
      const parserError = new Error('synthetic parser failure')
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      const splitMetadata = vi.spyOn(org, 'splitOrgMetadata')
      if (failurePoint === 'metadata') {
        splitMetadata.mockImplementation(() => {
          throw parserError
        })
      } else {
        mockEditorRender.mockImplementation(() => {
          throw parserError
        })
      }
      mockWritableHandle()
      vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
        value: '* Note',
        loading: false,
        error: null,
        retry: vi.fn(),
      })

      // The failed note retains its title and leaves notebook controls mounted.
      const view = (sourceRef: string, noteName = 'note.org') => (
        <>
          <nav aria-label="Notebook">
            <button type="button">Other note</button>
          </nav>
          <NoteContentView
            worldState={mockWorldState as never}
            sourceRef={sourceRef}
            noteName={noteName}
            editing={false}
            onToggleEdit={vi.fn()}
          />
        </>
      )
      const { rerender } = render(view('first/-/docs'))
      expect(screen.getByRole('alert').textContent).toBe(
        'Could not open this note. Select another note.',
      )
      expect(screen.getByRole('navigation', { name: 'Notebook' })).toBeDefined()
      expect(screen.getByRole('button', { name: 'Other note' })).toBeDefined()
      expect(screen.getByText('note')).toBeDefined()
      expect(screen.queryByText(parserError.message)).toBeNull()
      expect(consoleError).toHaveBeenCalled()

      // A different file or source starts with a fresh error boundary.
      splitMetadata.mockRestore()
      mockEditorRender.mockReset()
      rerender(view(nextSource, nextNote))
      expect(screen.queryByRole('alert')).toBeNull()
      expect(screen.getByTestId('lexical-editor')).toBeDefined()
    },
  )

  it('shows "Select a note to view" when noteName is empty', () => {
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName=""
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    expect(screen.getByText('Select a note to view')).toBeDefined()
  })

  it('shows loading state when text content is loading', () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })

    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="test.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    expect(screen.getByText('Loading…')).toBeDefined()
  })

  it('shows error state when text content fails to load', () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Permission denied'),
      retry: vi.fn(),
    })

    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="test.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    expect(screen.getByText('Failed to load note')).toBeDefined()
    expect(screen.getByText('Permission denied')).toBeDefined()
  })

  it('renders LexicalEditor in WYSIWYG mode (default)', () => {
    // Load a Markdown note without metadata.
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: '# Hello World\n\nSome content here.',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open the formatted note and check its editor content.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="hello.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    const editor = screen.getByTestId('lexical-editor')
    expect(editor).toBeDefined()
    expect(editor.getAttribute('data-content')).toBe(
      '# Hello World\n\nSome content here.',
    )
    expect(editor.getAttribute('data-format')).toBe('markdown')

    // Title should strip .md extension.
    expect(screen.getByText('hello')).toBeDefined()

    // Should show Source button in WYSIWYG mode.
    expect(screen.getByText('Source')).toBeDefined()
  })

  it('renders frontmatter display for notes with frontmatter', () => {
    // Load frontmatter alongside the note body.
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: '---\ntags: [alpha, beta]\n---\n\n# Note\n\nBody text.',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Keep the parsed frontmatter visible above the editor.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    const fm = screen.getByTestId('frontmatter-display')
    expect(fm).toBeDefined()
    expect(fm.textContent).toContain('alpha,beta')
  })

  it('opens Org metadata outside Lexical and saves through the shared writer', async () => {
    // Provide Org metadata and a writable file handle.
    const orgContent =
      '#+TITLE: Org Note\n#+SETUPFILE: ../../setup.org\n\n* TODO Heading\n:PROPERTIES:\n:CUSTOM_ID: h\n:END:\n'
    const writeAt = vi.fn(() => Promise.resolve(0n))
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: orgContent,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open the Org body in the editor.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.org"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    // Keep the metadata outside the editable body.
    const editor = screen.getByTestId('lexical-editor')
    expect(editor.getAttribute('data-format')).toBe('org')
    expect(editor.getAttribute('data-content')).toBe(
      '* TODO Heading\n:PROPERTIES:\n:CUSTOM_ID: h\n:END:\n',
    )
    expect(screen.queryByTestId('frontmatter-display')).toBeNull()
    expect(screen.getByText('note')).toBeDefined()

    // Save the edited body with the original metadata restored.
    const expectedContent =
      '#+TITLE: Org Note\n#+SETUPFILE: ../../setup.org\n\nsaved-content'
    const expectedEncoded = new TextEncoder().encode(expectedContent)
    fireEvent.click(screen.getByText('mock-save'))

    // Require both the write and final truncation to use complete bytes.
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())
    expect(writeAt).toHaveBeenCalledWith(0n, expectedEncoded)
    expect(truncate).toHaveBeenCalledWith(BigInt(expectedEncoded.byteLength))
  })

  it('saves untouched Org content byte-stable through the WYSIWYG path', async () => {
    // Provide an unchanged Org note and a writable handle.
    const orgContent =
      '#+TITLE: Org Note\n#+SETUPFILE: ../../setup.org\n\n* TODO Heading\n:PROPERTIES:\n:CUSTOM_ID: h\n:END:\n'
    const writeAt = vi.fn(() => Promise.resolve(0n))
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: orgContent,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open the original body in formatted mode.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.org"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    // Save the editor content without changing the note.
    fireEvent.click(screen.getByText('mock-save-current'))

    // Compare the written bytes with the complete original note.
    const expectedEncoded = new TextEncoder().encode(orgContent)
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())
    expect(writeAt).toHaveBeenCalledWith(0n, expectedEncoded)
    expect(truncate).toHaveBeenCalledWith(BigInt(expectedEncoded.byteLength))
  })

  it('renders textarea in source mode', () => {
    // Load a plain note for source editing.
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'Editable text',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open source mode and check its content and mode action.
    const { container } = render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )
    const textarea = container.querySelector('textarea')
    expect(textarea?.value).toBe('Editable text')

    // Should show WYSIWYG button in source mode.
    expect(screen.getByText('WYSIWYG')).toBeDefined()
  })

  it('calls onToggleEdit when Source button is clicked', () => {
    // Provide a writable note for the mode switch.
    mockWritableHandle()
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'content',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Switch from formatted mode to source mode.
    const onToggleEdit = vi.fn()
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={onToggleEdit}
      />,
    )
    fireEvent.click(screen.getByText('Source'))
    expect(onToggleEdit).toHaveBeenCalledOnce()
  })

  it('calls onToggleEdit when WYSIWYG button is clicked in source mode', () => {
    // Provide a writable source note.
    mockWritableHandle()
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'content',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Switch back to formatted mode.
    const onToggleEdit = vi.fn()
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={onToggleEdit}
      />,
    )
    fireEvent.click(screen.getByText('WYSIWYG'))
    expect(onToggleEdit).toHaveBeenCalledOnce()
  })

  it('waits for source content to save before leaving source mode', async () => {
    // Load the original source content.
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Hold the write until its completion is explicitly released.
    let resolveWriteAt: (() => void) | undefined
    const writeAt = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveWriteAt = resolve
        }),
    )
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open source mode with a tracked mode-switch callback.
    const onToggleEdit = vi.fn()
    const { container } = render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={onToggleEdit}
      />,
    )

    // Change the source and request formatted mode.
    const textarea = container.querySelector('textarea')!
    fireEvent.change(textarea, { target: { value: 'updated' } })
    fireEvent.click(screen.getByText('WYSIWYG'))

    // Keep the editor in source mode while the write remains pending.
    await waitFor(() =>
      expect(writeAt).toHaveBeenCalledWith(
        0n,
        new TextEncoder().encode('updated'),
      ),
    )
    expect(onToggleEdit).not.toHaveBeenCalled()
    expect(truncate).not.toHaveBeenCalled()

    // Complete the write and allow the mode switch.
    resolveWriteAt?.()

    // Require truncation before reporting the completed switch.
    await waitFor(() => expect(truncate).toHaveBeenCalledOnce())
    await waitFor(() => expect(onToggleEdit).toHaveBeenCalledOnce())
  })

  it('does not duplicate source saves when the WYSIWYG button blurs the editor', async () => {
    // Load the original source content.
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Provide a writable handle for the blur and click sequence.
    const writeAt = vi.fn(() => Promise.resolve())
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open source mode.
    const { container } = render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )

    // Reproduce the pointer, blur and click sequence for the mode switch.
    const textarea = container.querySelector('textarea')!
    const toggle = screen.getByText('WYSIWYG')
    fireEvent.change(textarea, { target: { value: 'updated' } })
    fireEvent.pointerDown(toggle)
    fireEvent.blur(textarea)
    fireEvent.click(toggle)

    // Require one write and one truncation for that switch.
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())
    expect(truncate).toHaveBeenCalledOnce()
  })

  it('updates textarea content on change in source mode', () => {
    // Load a source note.
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Change the source textarea and check its current value.
    const { container } = render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )
    const textarea = container.querySelector('textarea')!
    fireEvent.change(textarea, { target: { value: 'modified' } })
    expect(textarea.value).toBe('modified')
  })

  it('retries a failed note read through the resource', () => {
    // Expose a failed read with the resource retry action.
    const retry = vi.fn()
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Permission denied'),
      retry,
    })

    // Render the read failure.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="test.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    // Retry through the action shown beside the read error.
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('reports durable WYSIWYG completion and retries the retained draft', async () => {
    // Reject the first write and allow the retry.
    const writeAt = vi
      .fn<() => Promise<bigint>>()
      .mockRejectedValueOnce(new Error('disk full'))
      .mockResolvedValueOnce(0n)
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open the formatted note.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    // Save the edit and keep truncation withheld after the failed write.
    fireEvent.click(screen.getByText('mock-save'))
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toContain(
        'Failed to save note: disk full',
      ),
    )
    expect(truncate).not.toHaveBeenCalled()

    // Retry the retained edit and show its pending state.
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.getByRole('status').textContent).toBe('Saving…')

    // Require a complete retry before showing Saved.
    await waitFor(() =>
      expect(screen.getByRole('status').textContent).toBe('Saved'),
    )
    expect(writeAt).toHaveBeenCalledTimes(2)
    expect(truncate).toHaveBeenCalledOnce()
    expect(writeAt).toHaveBeenLastCalledWith(
      0n,
      new TextEncoder().encode('saved-content'),
    )
  })

  it('serializes a newer WYSIWYG edit behind a pending write', async () => {
    // Hold the first write while allowing the next write to complete.
    let resolveFirst: (() => void) | undefined
    const writeAt = vi
      .fn<() => Promise<void>>()
      .mockImplementationOnce(
        () =>
          new Promise<void>((resolve) => {
            resolveFirst = resolve
          }),
      )
      .mockResolvedValueOnce()
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open the formatted note.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    // Submit two edits while the first write is pending.
    fireEvent.click(screen.getByText('mock-save'))
    fireEvent.click(screen.getByText('mock-save-new'))
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())

    // Complete the first write and require the newer bytes to save next.
    resolveFirst?.()
    await waitFor(() => expect(writeAt).toHaveBeenCalledTimes(2))
    await waitFor(() =>
      expect(screen.getByRole('status').textContent).toBe('Saved'),
    )
    expect(writeAt).toHaveBeenNthCalledWith(
      2,
      0n,
      new TextEncoder().encode('newer-content'),
    )
  })

  it('does not publish pending completion after unmount', async () => {
    // Hold the write and track completion notifications.
    let resolveWrite: (() => void) | undefined
    const writeAt = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveWrite = resolve
        }),
    )
    const onContentSaved = vi.fn()
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate: vi.fn(() => Promise.resolve()) } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    const view = render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
        onContentSaved={onContentSaved}
      />,
    )

    // Unmount the view before resolving its pending write.
    fireEvent.click(screen.getByText('mock-save'))
    view.unmount()
    resolveWrite?.()
    await Promise.resolve()
    await Promise.resolve()
    expect(onContentSaved).not.toHaveBeenCalled()
  })

  it('isolates source and equal-content editor state by note path', async () => {
    // Load two note paths with identical saved content.
    mockWritableHandle()
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'equal content',
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    const view = render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="a.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )
    fireEvent.change(screen.getByLabelText('Note source'), {
      target: { value: 'A draft' },
    })

    // Switch paths and require the new note to discard the old source draft.
    view.rerender(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="b.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )
    await waitFor(() =>
      expect(
        (screen.getByLabelText('Note source') as HTMLTextAreaElement).value,
      ).toBe('equal content'),
    )
  })

  it('retains the draft and withholds completion when truncate rejects', async () => {
    // Allow the write but reject its final truncation.
    const truncate = vi.fn(() => Promise.reject(new Error('truncate failed')))
    const onContentSaved = vi.fn()
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt: vi.fn(() => Promise.resolve()), truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
        onContentSaved={onContentSaved}
      />,
    )

    // Keep the failed edit retryable and withhold completion.
    fireEvent.click(screen.getByText('mock-save'))
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toContain(
        'truncate failed',
      ),
    )
    expect(onContentSaved).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDefined()
  })

  it('retries the newest source draft after a failed write', async () => {
    // Reject the first source write and allow the retry.
    const writeAt = vi
      .fn<() => Promise<void>>()
      .mockRejectedValueOnce(new Error('disk full'))
      .mockResolvedValueOnce()
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: {
        writeAt,
        truncate: vi.fn(() => Promise.resolve()),
      } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )

    // Submit the first source draft and wait for its failure.
    const editor = screen.getByLabelText('Note source')
    fireEvent.change(editor, { target: { value: 'failed draft' } })
    fireEvent.blur(editor)
    await waitFor(() => expect(screen.getByRole('alert')).toBeDefined())

    // Change the failed draft before retrying and require the newest bytes.
    fireEvent.change(editor, { target: { value: 'newest draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(writeAt).toHaveBeenCalledTimes(2))
    expect(writeAt).toHaveBeenLastCalledWith(
      0n,
      new TextEncoder().encode('newest draft'),
    )
  })

  it('keeps the WYSIWYG editor mounted when a newer edit saves after X', async () => {
    // Retain each pending write for explicit completion.
    const resolvers: Array<() => void> = []
    const writeAt = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolvers.push(resolve)
        }),
    )
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: {
        writeAt,
        truncate: vi.fn(() => Promise.resolve()),
      } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Mount the formatted editor before submitting edits.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    const editor = screen.getByTestId('lexical-editor')
    const initialKey = editor.getAttribute('data-composer-key')

    // Queue two edits and finish their writes in order.
    fireEvent.click(screen.getByText('mock-save'))
    fireEvent.click(screen.getByText('mock-save-new'))
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())
    resolvers[0]?.()
    await waitFor(() => expect(writeAt).toHaveBeenCalledTimes(2))
    resolvers[1]?.()

    // Require the existing editor instance to survive both completions.
    await waitFor(() =>
      expect(screen.getByRole('status').textContent).toBe('Saved'),
    )
    expect(
      screen.getByTestId('lexical-editor').getAttribute('data-composer-key'),
    ).toBe(initialKey)
  })

  it('keeps unsent source Y when Retry X resolves', async () => {
    // Reject the first write and hold its retry.
    let resolveRetry: (() => void) | undefined
    const writeAt = vi
      .fn<() => Promise<void>>()
      .mockRejectedValueOnce(new Error('disk full'))
      .mockImplementationOnce(
        () =>
          new Promise<void>((resolve) => {
            resolveRetry = resolve
          }),
      )
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate: vi.fn(() => Promise.resolve()) } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Mount the source editor before submitting the failed draft.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )

    // Submit X, retry it and type Y before the retry completes.
    const editor = screen.getByLabelText('Note source')
    fireEvent.change(editor, { target: { value: 'X' } })
    fireEvent.blur(editor)
    await waitFor(() => expect(screen.getByRole('alert')).toBeDefined())
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(writeAt).toHaveBeenCalledTimes(2))
    fireEvent.change(editor, { target: { value: 'Y' } })
    resolveRetry?.()

    // Require the newer unsent draft to survive the older retry.
    await waitFor(() => expect(screen.queryByText('Saving…')).toBeNull())
    expect(screen.queryByText('Saved')).toBeNull()
    expect(
      (screen.getByLabelText('Note source') as HTMLTextAreaElement).value,
    ).toBe('Y')
  })

  it('keeps source Y and stays in source mode when toggle save X resolves', async () => {
    // Hold the source write during a requested mode switch.
    let resolveWrite: (() => void) | undefined
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: {
        writeAt: vi.fn(
          () =>
            new Promise<void>((resolve) => {
              resolveWrite = resolve
            }),
        ),
        truncate: vi.fn(() => Promise.resolve()),
      } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Mount source mode with a tracked mode-switch callback.
    const onToggleEdit = vi.fn()
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={onToggleEdit}
      />,
    )

    // Request a switch for X, then type Y while X is still saving.
    const editor = screen.getByLabelText('Note source')
    fireEvent.change(editor, { target: { value: 'X' } })
    fireEvent.click(screen.getByText('WYSIWYG'))
    await waitFor(() => expect(resolveWrite).toBeDefined())
    fireEvent.change(editor, { target: { value: 'Y' } })
    resolveWrite?.()

    // Keep the newer source draft and withhold the stale mode switch.
    await waitFor(() => expect(screen.queryByText('Saving…')).toBeNull())
    expect(screen.queryByText('Saved')).toBeNull()
    expect(onToggleEdit).not.toHaveBeenCalled()
    expect(
      (screen.getByLabelText('Note source') as HTMLTextAreaElement).value,
    ).toBe('Y')
  })

  it('keeps Saved across an editor update that exports identical text', async () => {
    // Provide a writable formatted note.
    const writeAt = vi.fn(() => Promise.resolve(0n))
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    // Save one edit and wait for durable completion.
    fireEvent.click(screen.getByText('mock-save'))
    await waitFor(() =>
      expect(screen.getByRole('status').textContent).toBe('Saved'),
    )

    // An editor update that re-exports the saved text (mount, mode toggle)
    // is not an edit and must leave the completed save visible.
    fireEvent.click(screen.getByText('mock-save-current'))
    expect(screen.getByRole('status').textContent).toBe('Saved')
    expect(screen.queryByText('Saving…')).toBeNull()
    await waitFor(() => expect(truncate).toHaveBeenCalledOnce())
    expect(screen.getByRole('status').textContent).toBe('Saved')
  })

  it('does not announce Saved for X after unsent WYSIWYG Y drafts', async () => {
    // Hold the formatted edit while a newer unsent draft arrives.
    let resolveWrite: (() => void) | undefined
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: {
        writeAt: vi.fn(
          () =>
            new Promise<void>((resolve) => {
              resolveWrite = resolve
            }),
        ),
        truncate: vi.fn(() => Promise.resolve()),
      } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Mount the formatted note.
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    // Save X, then retain unsent Y before X completes.
    fireEvent.click(screen.getByText('mock-save'))
    await waitFor(() => expect(resolveWrite).toBeDefined())
    fireEvent.click(screen.getByText('mock-draft-new'))
    resolveWrite?.()
    await waitFor(() => expect(screen.queryByText('Saving…')).toBeNull())
    expect(screen.queryByText('Saved')).toBeNull()
  })

  it('keeps note B clean when note A completes writing after the switch', async () => {
    // Hold one note write and track completion notifications.
    let resolveWrite: (() => void) | undefined
    const writeAt = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveWrite = resolve
        }),
    )
    const truncate = vi.fn(() => Promise.resolve())
    const onContentSaved = vi.fn()
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    // Open note A with a tracked completion callback.
    const view = render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="a.md"
        editing={false}
        onToggleEdit={vi.fn()}
        onContentSaved={onContentSaved}
      />,
    )

    // Submit A and wait for its write to start.
    fireEvent.click(screen.getByText('mock-save'))
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())

    // Switch notes while A's write is still in flight.
    view.rerender(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="b.md"
        editing={false}
        onToggleEdit={vi.fn()}
        onContentSaved={onContentSaved}
      />,
    )

    // Complete A after B has mounted.
    resolveWrite?.()
    await waitFor(() => expect(truncate).toHaveBeenCalledOnce())

    // Drain microtasks plus scheduler macrotasks so any leaked completion
    // would have rendered before the absence checks below.
    for (let i = 0; i < 20; i++) {
      await Promise.resolve()
      if (i % 4 === 3) {
        await new Promise((resolve) => setTimeout(resolve, 0))
      }
    }

    // A's completion must not publish through B's view.
    expect(screen.queryByRole('status')).toBeNull()
    expect(screen.queryByText('Saving…')).toBeNull()
    expect(screen.queryByText('Saved')).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(onContentSaved).not.toHaveBeenCalled()

    // B's own content and editor stay intact.
    const editor = screen.getByTestId('lexical-editor')
    expect(editor.getAttribute('data-content')).toBe('original')
    expect(editor.getAttribute('data-composer-key')).toBe('docs/b.md:markdown')
  })
})

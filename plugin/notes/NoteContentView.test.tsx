import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  render,
  screen,
  cleanup,
  fireEvent,
  waitFor,
} from '@testing-library/react'

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
    onDirty,
  }: {
    content: string
    format: string
    onSave: (content: string) => Promise<void>
    composerKey?: string
    onDraftChange?: (content: string) => void
    onDirty?: () => void
  }) => (
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
      <button type="button" onClick={onDirty}>
        mock-dirty
      </button>
    </div>
  ),
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
  })

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
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: '# Hello World\n\nSome content here.',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

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
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: '---\ntags: [alpha, beta]\n---\n\n# Note\n\nBody text.',
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
    const fm = screen.getByTestId('frontmatter-display')
    expect(fm).toBeDefined()
    expect(fm.textContent).toContain('alpha,beta')
  })

  it('opens Org metadata outside Lexical and saves through the shared writer', async () => {
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

    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.org"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    const editor = screen.getByTestId('lexical-editor')
    expect(editor.getAttribute('data-format')).toBe('org')
    expect(editor.getAttribute('data-content')).toBe(
      '* TODO Heading\n:PROPERTIES:\n:CUSTOM_ID: h\n:END:\n',
    )
    expect(screen.queryByTestId('frontmatter-display')).toBeNull()
    expect(screen.getByText('note')).toBeDefined()

    const expectedContent =
      '#+TITLE: Org Note\n#+SETUPFILE: ../../setup.org\n\nsaved-content'
    const expectedEncoded = new TextEncoder().encode(expectedContent)

    fireEvent.click(screen.getByText('mock-save'))

    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())
    expect(writeAt).toHaveBeenCalledWith(0n, expectedEncoded)
    expect(truncate).toHaveBeenCalledWith(BigInt(expectedEncoded.byteLength))
  })

  it('saves untouched Org content byte-stable through the WYSIWYG path', async () => {
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

    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.org"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('mock-save-current'))

    const expectedEncoded = new TextEncoder().encode(orgContent)
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())
    expect(writeAt).toHaveBeenCalledWith(0n, expectedEncoded)
    expect(truncate).toHaveBeenCalledWith(BigInt(expectedEncoded.byteLength))
  })

  it('renders textarea in source mode', () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'Editable text',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

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
    expect(textarea).toBeDefined()
    expect(textarea!.value).toBe('Editable text')
    // Should show WYSIWYG button in source mode.
    expect(screen.getByText('WYSIWYG')).toBeDefined()
  })

  it('calls onToggleEdit when Source button is clicked', () => {
    mockWritableHandle()
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'content',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

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
    mockWritableHandle()
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'content',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

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
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

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

    const textarea = container.querySelector('textarea')!
    fireEvent.change(textarea, { target: { value: 'updated' } })
    fireEvent.click(screen.getByText('WYSIWYG'))

    await waitFor(() =>
      expect(writeAt).toHaveBeenCalledWith(
        0n,
        new TextEncoder().encode('updated'),
      ),
    )
    expect(onToggleEdit).not.toHaveBeenCalled()
    expect(truncate).not.toHaveBeenCalled()

    resolveWriteAt?.()

    await waitFor(() => expect(truncate).toHaveBeenCalledOnce())
    await waitFor(() => expect(onToggleEdit).toHaveBeenCalledOnce())
  })

  it('does not duplicate source saves when the WYSIWYG button blurs the editor', async () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    const writeAt = vi.fn(() => Promise.resolve())
    const truncate = vi.fn(() => Promise.resolve())
    vi.mocked(useUnixFSHandle).mockReturnValue({
      value: { writeAt, truncate } as never,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

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
    const toggle = screen.getByText('WYSIWYG')
    fireEvent.change(textarea, { target: { value: 'updated' } })
    fireEvent.pointerDown(toggle)
    fireEvent.blur(textarea)
    fireEvent.click(toggle)

    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())
    expect(truncate).toHaveBeenCalledOnce()
  })

  it('updates textarea content on change in source mode', () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    })

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
    const retry = vi.fn()
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Permission denied'),
      retry,
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

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('reports durable WYSIWYG completion and retries the retained draft', async () => {
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

    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('mock-save'))
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toContain(
        'Failed to save note: disk full',
      ),
    )
    expect(truncate).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.getByRole('status').textContent).toBe('Saving…')

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

    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByText('mock-save'))
    fireEvent.click(screen.getByText('mock-save-new'))
    await waitFor(() => expect(writeAt).toHaveBeenCalledOnce())

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

    fireEvent.click(screen.getByText('mock-save'))
    view.unmount()
    resolveWrite?.()
    await Promise.resolve()
    await Promise.resolve()
    expect(onContentSaved).not.toHaveBeenCalled()
  })

  it('isolates source and equal-content editor state by note path', async () => {
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

    const editor = screen.getByLabelText('Note source')
    fireEvent.change(editor, { target: { value: 'failed draft' } })
    fireEvent.blur(editor)
    await waitFor(() => expect(screen.getByRole('alert')).toBeDefined())

    fireEvent.change(editor, { target: { value: 'newest draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(writeAt).toHaveBeenCalledTimes(2))
    expect(writeAt).toHaveBeenLastCalledWith(
      0n,
      new TextEncoder().encode('newest draft'),
    )
  })
  it('keeps the WYSIWYG editor mounted when X saves after unsent Y', async () => {
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
    fireEvent.click(screen.getByText('mock-save'))
    fireEvent.click(screen.getByText('mock-draft-new'))
    await waitFor(() => expect(resolveWrite).toBeDefined())
    resolveWrite?.()

    await waitFor(() =>
      expect(screen.getByRole('status').textContent).toBe('Saved'),
    )
    expect(
      screen.getByTestId('lexical-editor').getAttribute('data-composer-key'),
    ).toBe(initialKey)
  })

  it('keeps unsent source Y when Retry X resolves', async () => {
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
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={true}
        onToggleEdit={vi.fn()}
      />,
    )
    const editor = screen.getByLabelText('Note source')
    fireEvent.change(editor, { target: { value: 'X' } })
    fireEvent.blur(editor)
    await waitFor(() => expect(screen.getByRole('alert')).toBeDefined())
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(writeAt).toHaveBeenCalledTimes(2))
    fireEvent.change(editor, { target: { value: 'Y' } })
    resolveRetry?.()

    await waitFor(() => expect(screen.queryByText('Saving…')).toBeNull())
    expect(screen.queryByText('Saved')).toBeNull()
    expect(
      (screen.getByLabelText('Note source') as HTMLTextAreaElement).value,
    ).toBe('Y')
  })
  it('keeps source Y and stays in source mode when toggle save X resolves', async () => {
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
    const editor = screen.getByLabelText('Note source')
    fireEvent.change(editor, { target: { value: 'X' } })
    fireEvent.click(screen.getByText('WYSIWYG'))
    await waitFor(() => expect(resolveWrite).toBeDefined())
    fireEvent.change(editor, { target: { value: 'Y' } })
    resolveWrite?.()

    await waitFor(() => expect(screen.queryByText('Saving…')).toBeNull())
    expect(screen.queryByText('Saved')).toBeNull()
    expect(onToggleEdit).not.toHaveBeenCalled()
    expect(
      (screen.getByLabelText('Note source') as HTMLTextAreaElement).value,
    ).toBe('Y')
  })

  it('does not announce Saved for X after unsent WYSIWYG Y dirties the editor', async () => {
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
    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="note.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByText('mock-save'))
    await waitFor(() => expect(resolveWrite).toBeDefined())
    fireEvent.click(screen.getByText('mock-dirty'))
    resolveWrite?.()
    await waitFor(() => expect(screen.queryByText('Saving…')).toBeNull())
    expect(screen.queryByText('Saved')).toBeNull()
  })

  it('keeps note B clean when note A completes writing after the switch', async () => {
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

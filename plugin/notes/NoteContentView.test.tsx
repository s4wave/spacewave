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
  }: {
    content: string
    format: string
    onSave: (content: string) => void
  }) => (
    <div
      data-testid="lexical-editor"
      data-content={content}
      data-format={format}
    >
      <button type="button" onClick={() => onSave('saved-content')}>
        mock-save
      </button>
      <button type="button" onClick={() => onSave(content)}>
        mock-save-current
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

    expect(writeAt).toHaveBeenCalledWith(
      0n,
      new TextEncoder().encode('updated'),
    )
    expect(onToggleEdit).not.toHaveBeenCalled()

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
})

import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'

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
  default: ({ markdown, onSave }: { markdown: string; onSave: (md: string) => void }) => (
    <div data-testid="lexical-editor" data-markdown={markdown}>
      <button type="button" onClick={() => onSave('saved-content')}>
        mock-save
      </button>
    </div>
  ),
}))

vi.mock('./FrontmatterDisplay.js', () => ({
  default: ({ frontmatter }: { frontmatter: Record<string, unknown> }) => (
    <div data-testid="frontmatter-display">
      {frontmatter.tags ? `tags: ${(frontmatter.tags as string[]).join(',')}` : null}
    </div>
  ),
}))

import NoteContentView from './NoteContentView.js'
import { useUnixFSHandleTextContent } from '@s4wave/web/hooks/useUnixFSHandle.js'

const mockWorldState = {
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
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
    } as never)

    render(
      <NoteContentView
        worldState={mockWorldState as never}
        sourceRef="obj-key/-/docs"
        noteName="test.md"
        editing={false}
        onToggleEdit={vi.fn()}
      />,
    )
    expect(screen.getByText('Loading...')).toBeDefined()
  })

  it('shows error state when text content fails to load', () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Permission denied'),
      retry: vi.fn(),
    } as never)

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
    } as never)

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
    expect(editor.getAttribute('data-markdown')).toBe(
      '# Hello World\n\nSome content here.',
    )
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
    } as never)

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

  it('renders textarea in source mode', () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'Editable text',
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)

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
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'content',
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)

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
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'content',
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)

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

  it('updates textarea content on change in source mode', () => {
    vi.mocked(useUnixFSHandleTextContent).mockReturnValue({
      value: 'original',
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)

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

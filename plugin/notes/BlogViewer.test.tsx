import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import { StateNamespaceProvider, atom } from '@s4wave/web/state/index.js'

const mockUseWorldObjectMessageState = vi.hoisted(() => vi.fn())
const mockUseUnixFSRootHandle = vi.hoisted(() => vi.fn())
const mockUseUnixFSHandle = vi.hoisted(() => vi.fn())
const mockUseUnixFSHandleEntries = vi.hoisted(() => vi.fn())
const mockUseBlogPosts = vi.hoisted(() => vi.fn())
const mockUseAuthorRegistry = vi.hoisted(() => vi.fn())
const mockParseObjectUri = vi.hoisted(() => vi.fn())

vi.mock('./useWorldObjectMessageState.js', () => ({
  useWorldObjectMessageState: mockUseWorldObjectMessageState,
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: vi.fn(() => 'blog/blog'),
}))

vi.mock('@s4wave/web/object/ViewerStatusShell.js', () => ({
  ViewerStatusShell: ({ children }: { children: React.ReactNode }) => children,
}))

vi.mock('@s4wave/sdk/space/object-uri.js', () => ({
  parseObjectUri: mockParseObjectUri,
}))

vi.mock('@s4wave/web/hooks/useUnixFSHandle.js', () => ({
  useUnixFSRootHandle: mockUseUnixFSRootHandle,
  useUnixFSHandle: mockUseUnixFSHandle,
  useUnixFSHandleEntries: mockUseUnixFSHandleEntries,
}))

vi.mock('./blog/useBlogPosts.js', () => ({
  useBlogPosts: mockUseBlogPosts,
}))

vi.mock('./blog/authors.js', () => ({
  useAuthorRegistry: mockUseAuthorRegistry,
  resolveAuthor: vi.fn(() => null),
}))

vi.mock('./blog/BlogReadingView.js', () => ({
  BlogReadingView: () => <div data-testid="blog-reading-view" />,
}))

vi.mock('./NoteContentView.js', () => ({
  default: () => <div data-testid="note-content-view" />,
}))

vi.mock('./NoteList.js', () => ({
  default: ({
    onCreateNote,
  }: {
    onCreateNote?: () => void
  }) => (
    <button title="New note" onClick={onCreateNote}>
      New note
    </button>
  ),
}))

import BlogViewer from './BlogViewer.js'
import { useUnixFSHandleEntries } from '@s4wave/web/hooks/useUnixFSHandle.js'
import { useBlogPosts } from './blog/useBlogPosts.js'
import { useAuthorRegistry } from './blog/authors.js'

function buildResource<T>(value: T) {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

const mockWorldState = buildResource(null)
const rootHandle = buildResource({ id: 'root-handle' })

function renderViewer() {
  const rootAtom = atom<Record<string, unknown>>({})
  return render(
    <StateNamespaceProvider rootAtom={rootAtom} namespace={['blog-viewer-test']}>
      <BlogViewer
        objectInfo={{} as never}
        worldState={mockWorldState as never}
      />
    </StateNamespaceProvider>,
  )
}

describe('BlogViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('uses the resolved source subpath handle for reading-mode entries and authors', () => {
    const pathHandle = buildResource({ id: 'path-handle' })
    const entries = buildResource([])

    mockUseWorldObjectMessageState.mockReturnValue({
      state: buildResource({ name: 'Blog', authorRegistryPath: 'authors.yaml' }),
      sources: [{ name: 'Posts', ref: 'blog-fs/-/nested/posts' }],
    })
    mockParseObjectUri.mockReturnValue({
      objectKey: 'blog-fs',
      path: 'nested/posts',
    })
    mockUseUnixFSRootHandle.mockReturnValue(rootHandle)
    mockUseUnixFSHandle.mockReturnValue(pathHandle)
    mockUseUnixFSHandleEntries.mockReturnValue(entries)
    mockUseBlogPosts.mockReturnValue(buildResource([]))
    mockUseAuthorRegistry.mockReturnValue(buildResource({}))

    renderViewer()

    expect(useUnixFSHandleEntries).toHaveBeenCalledWith(pathHandle, {
      enabled: true,
    })
    expect(useBlogPosts).toHaveBeenCalledWith(pathHandle, entries)
    expect(useAuthorRegistry).toHaveBeenCalledWith(pathHandle, 'authors.yaml')
  })

  it('includes author frontmatter in the new-post template', async () => {
    const child = {
      writeAt: vi.fn(() => Promise.resolve(0n)),
      release: vi.fn(),
    }
    const handle = {
      mknod: vi.fn(() => Promise.resolve()),
      lookup: vi.fn(() => Promise.resolve(child)),
    }
    const pathHandle = buildResource(handle)
    const entries = buildResource([])

    mockUseWorldObjectMessageState.mockReturnValue({
      state: buildResource({ name: 'Blog', authorRegistryPath: '' }),
      sources: [{ name: 'Posts', ref: 'blog-fs/-/' }],
    })
    mockParseObjectUri.mockReturnValue({
      objectKey: 'blog-fs',
      path: '',
    })
    mockUseUnixFSRootHandle.mockReturnValue(rootHandle)
    mockUseUnixFSHandle.mockReturnValue(pathHandle)
    mockUseUnixFSHandleEntries.mockReturnValue(entries)
    mockUseBlogPosts.mockReturnValue(buildResource([]))
    mockUseAuthorRegistry.mockReturnValue(buildResource({}))

    renderViewer()

    fireEvent.click(screen.getByTitle('Editing mode'))
    fireEvent.click(screen.getByTitle('New note'))

    await waitFor(() => {
      expect(handle.mknod).toHaveBeenCalledWith(['new-post.md'], expect.anything())
      expect(child.writeAt).toHaveBeenCalled()
    })

    const writeCall = child.writeAt.mock.calls[0] as unknown as [
      bigint,
      Uint8Array,
    ]
    const encoded = writeCall[1]
    const text = new TextDecoder().decode(encoded)
    expect(text).toContain('author: \n')
    expect(text).toContain('draft: true\n')
  })
})

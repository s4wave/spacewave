import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
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
    value: [],
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

import NotebookViewer from './NotebookViewer.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'

const mockObjectInfo = { key: 'mock-key', typeId: 'spacewave-notes/notebook' }
const mockWorldState = {
  value: null,
  loading: false,
  error: null,
  retry: vi.fn(),
}

function renderViewer() {
  const rootAtom = atom<Record<string, unknown>>({})
  return render(
    <StateNamespaceProvider rootAtom={rootAtom} namespace={['viewer-test']}>
      <NotebookViewer
        objectInfo={mockObjectInfo as never}
        worldState={mockWorldState as never}
      />
    </StateNamespaceProvider>,
  )
}

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

  it('renders loading state when notebook resource is loading', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    } as never)
    renderViewer()
    expect(screen.getByText('Loading notebook...')).toBeDefined()
  })

  it('renders loading state when notebook state is loading', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    } as never)
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: null,
        loading: true,
        error: null,
        retry: vi.fn(),
      },
      sources: [],
    })

    renderViewer()
    expect(screen.getByText('Loading notebook...')).toBeDefined()
  })

  it('renders error state when notebook resource has error', () => {
    vi.mocked(useAccessTypedHandle).mockReturnValue({
      value: null,
      loading: false,
      error: new Error('Connection refused'),
      retry: vi.fn(),
    } as never)
    mockUseWorldObjectMessageState.mockReturnValue({
      state: {
        value: null,
        loading: false,
        error: null,
        retry: vi.fn(),
      },
      sources: [],
    })

    renderViewer()
    expect(screen.getByText('Connection refused')).toBeDefined()
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
    expect(screen.getByText('My Docs')).toBeDefined()
  })
})

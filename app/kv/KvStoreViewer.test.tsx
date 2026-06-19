import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

const mockUseAccessTypedHandle = vi.hoisted(() => vi.fn())
const mockUseResource = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: mockUseAccessTypedHandle,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: mockUseResource,
}))

vi.mock('@s4wave/web/object/object.js', () => ({
  getObjectKey: () => 'kv/test-store',
}))

import { KvStoreViewer } from './KvStoreViewer.js'

describe('KvStoreViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows the key count from the typed handle', () => {
    mockUseAccessTypedHandle.mockReturnValue({
      value: { keyCount: vi.fn() },
      loading: false,
      error: null,
    })
    mockUseResource.mockReturnValue({
      value: 3n,
      loading: false,
      error: null,
      retry: vi.fn(),
    })

    render(<KvStoreViewer objectInfo={{} as never} worldState={{} as never} />)

    expect(screen.getByText('Key/Value Store')).toBeTruthy()
    expect(screen.getByText('3')).toBeTruthy()
    expect(screen.getByText('keys')).toBeTruthy()
  })

  it('shows a retryable error state when key count loading fails', () => {
    mockUseAccessTypedHandle.mockReturnValue({
      value: null,
      loading: false,
      error: null,
    })
    mockUseResource.mockReturnValue({
      value: undefined,
      loading: false,
      error: new Error('boom'),
      retry: vi.fn(),
    })

    render(<KvStoreViewer objectInfo={{} as never} worldState={{} as never} />)

    expect(screen.getByText('KV store unavailable')).toBeTruthy()
    expect(screen.getByText('Error: boom')).toBeTruthy()
  })
})

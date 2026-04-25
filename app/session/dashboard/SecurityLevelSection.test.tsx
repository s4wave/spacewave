import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { SecurityLevelSection } from './SecurityLevelSection.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Account } from '@s4wave/sdk/account/account.js'

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: vi.fn(),
}))

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

const mockUseStreamingResource = useStreamingResource as ReturnType<
  typeof vi.fn
>

function makeAccountResource(value: Account | null = null): Resource<Account> {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

describe('SecurityLevelSection', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  beforeEach(() => {
    mockUseStreamingResource.mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })
  })

  // mockAccountInfo sets the single watchAccountInfo resource for the section.
  function mockAccountInfo(resource: {
    value: unknown
    loading: boolean
    error: unknown
    retry: ReturnType<typeof vi.fn>
  }) {
    mockUseStreamingResource.mockReturnValueOnce(resource)
  }

  it('renders "Security Level" heading', () => {
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    expect(screen.getByText('Security Level')).toBeDefined()
  })

  it('renders loading message when loading', () => {
    mockUseStreamingResource.mockReturnValue({
      value: null,
      loading: true,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    expect(screen.getByText('Loading security info...')).toBeDefined()
  })

  it('renders nothing when only one keypair exists', () => {
    mockAccountInfo({
      value: { authThreshold: 0, keypairCount: 1 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    expect(screen.queryByText('Security Level')).toBeNull()
  })

  it('renders nothing when zero keypairs exist', () => {
    mockAccountInfo({
      value: { authThreshold: 0, keypairCount: 0 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    expect(screen.queryByText('Security Level')).toBeNull()
  })

  it('displays "Standard" level when threshold is 0 with multiple keypairs', () => {
    mockAccountInfo({
      value: { authThreshold: 0, keypairCount: 3 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    const standardElements = screen.getAllByText('Standard')
    expect(standardElements.length).toBeGreaterThanOrEqual(1)
    expect(
      screen.getByText('Any single auth method can authorize account changes.'),
    ).toBeDefined()
  })

  it('displays threshold requirement as "N of M required"', () => {
    mockAccountInfo({
      value: { authThreshold: 1, keypairCount: 3 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    expect(screen.getByText('2 of 3 required')).toBeDefined()
  })

  it('displays "Enhanced" level for mid-range threshold', () => {
    mockAccountInfo({
      value: { authThreshold: 1, keypairCount: 3 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    const enhancedElements = screen.getAllByText('Enhanced')
    expect(enhancedElements.length).toBeGreaterThanOrEqual(1)
  })

  it('displays "Maximum" level when threshold equals count - 1', () => {
    mockAccountInfo({
      value: { authThreshold: 2, keypairCount: 3 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    const maxElements = screen.getAllByText('Maximum')
    expect(maxElements.length).toBeGreaterThanOrEqual(1)
    expect(
      screen.getByText('All auth methods are required for account changes.'),
    ).toBeDefined()
  })

  it('renders security level options for multiple keypairs', () => {
    mockAccountInfo({
      value: { authThreshold: 0, keypairCount: 3 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    expect(screen.getByText('Any one method')).toBeDefined()
    expect(screen.getByText('2 methods required')).toBeDefined()
    expect(screen.getByText('All methods required')).toBeDefined()
  })

  it('shows re-authentication note for multiple keypairs', () => {
    mockAccountInfo({
      value: { authThreshold: 0, keypairCount: 2 },
      loading: false,
      error: null,
      retry: vi.fn(),
    })
    render(
      <SecurityLevelSection account={makeAccountResource({} as Account)} />,
    )
    expect(
      screen.getByText(
        'Changing security level requires account re-authentication.',
      ),
    ).toBeDefined()
  })
})

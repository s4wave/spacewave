import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ChangelogReleaseRoute } from './ChangelogReleaseRoute.js'

const mockParams = vi.hoisted(() => vi.fn())
const mockUseResource = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/router/router.js', () => ({
  useParams: () => mockParams(),
}))

vi.mock('@s4wave/web/router/NavigatePath.js', () => ({
  NavigatePath: ({ to }: { to: string }) => <div>redirect:{to}</div>,
}))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (...args: unknown[]) => {
    const resource: unknown = mockUseResource(...args)
    return resource
  },
}))

vi.mock('@s4wave/web/platform/detect-platform.js', () => ({
  detectPlatform: () => ({ os: 'macos', arch: 'arm64' }),
}))

vi.mock('./useLandingBackNavigation.js', () => ({
  useLandingBackNavigation: () => vi.fn(),
}))

vi.mock('./LegalFooter.js', () => ({
  LegalFooter: () => <div>footer</div>,
}))

describe('ChangelogReleaseRoute', () => {
  beforeEach(() => {
    cleanup()
    mockParams.mockReset()
    mockUseResource.mockReset()
  })
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows a loading card while the changelog resolves', () => {
    mockParams.mockReturnValue({ version: 'v0.53.1' })
    mockUseResource.mockReturnValue({ loading: true })

    render(<ChangelogReleaseRoute />)
    expect(screen.getByText('Loading release')).toBeTruthy()
  })

  it('renders the matching release, stripping the v prefix from the param', () => {
    mockParams.mockReturnValue({ version: 'v0.53.1' })
    mockUseResource.mockReturnValue({
      loading: false,
      value: {
        releases: [
          { version: '0.53.1', date: '2026-07-05', summary: 'Latest' },
        ],
      },
    })

    render(<ChangelogReleaseRoute />)
    expect(
      screen.getByRole('heading', { level: 1, name: 'Spacewave v0.53.1' }),
    ).toBeTruthy()
  })

  it('redirects to /changelog when the version is unknown', () => {
    mockParams.mockReturnValue({ version: 'v9.9.9' })
    mockUseResource.mockReturnValue({
      loading: false,
      value: {
        releases: [{ version: '0.53.1' }],
      },
    })

    render(<ChangelogReleaseRoute />)
    expect(screen.getByText('redirect:/changelog')).toBeTruthy()
  })
})

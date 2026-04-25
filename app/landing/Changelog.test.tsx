import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { Changelog } from './Changelog.js'

const mockGoBack = vi.hoisted(() => vi.fn())
const mockUseResource = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => 'root-resource',
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: (...args: unknown[]) => mockUseResource(...args),
}))

vi.mock('./useLandingBackNavigation.js', () => ({
  useLandingBackNavigation: () => mockGoBack,
}))

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => <div data-testid="shooting-stars" />,
}))

vi.mock('./LegalFooter.js', () => ({
  LegalFooter: () => <div>footer</div>,
}))

describe('Changelog', () => {
  beforeEach(() => {
    cleanup()
    mockGoBack.mockReset()
    mockUseResource.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders generated markdown links and hides release links when absent', () => {
    mockUseResource.mockReturnValue({
      loading: false,
      value: {
        releases: [
          {
            version: '0.2.0',
            date: '2026-04-17',
            summary: 'Summary fallback',
            summaryMarkdown:
              'Summary with [launch notes](https://example.com/releases/0.2.0).',
            features: [
              {
                description: 'Fallback feature',
                descriptionMarkdown:
                  'Feature with [docs](https://example.com/docs).',
              },
            ],
          },
        ],
      },
    })

    render(<Changelog />)

    expect(
      screen.getByRole('heading', { level: 2, name: 'v0.2.0' }),
    ).toBeTruthy()
    const summaryLink = screen.getByRole('link', { name: 'launch notes' })
    expect(summaryLink.getAttribute('href')).toBe(
      'https://example.com/releases/0.2.0',
    )
    const entryLink = screen.getByRole('link', { name: 'docs' })
    expect(entryLink.getAttribute('href')).toBe('https://example.com/docs')
    expect(screen.queryByTitle('View release')).toBeNull()
  })

  it('shows the release link only when the artifact provides one', () => {
    mockUseResource.mockReturnValue({
      loading: false,
      value: {
        releases: [
          {
            version: '0.2.0',
            summary: 'Summary fallback',
            summaryMarkdown: 'Summary fallback',
            releaseUrl:
              'https://github.com/s4wave/spacewave/releases/tag/v0.2.0',
          },
        ],
      },
    })

    render(<Changelog />)

    const releaseLink = screen.getByTitle('View release')
    expect(releaseLink.getAttribute('href')).toBe(
      'https://github.com/s4wave/spacewave/releases/tag/v0.2.0',
    )
  })

  it('uses the landing back navigation callback', () => {
    mockUseResource.mockReturnValue({
      loading: false,
      value: { releases: [] },
    })

    render(<Changelog />)
    fireEvent.click(screen.getByRole('button', { name: /back/i }))
    expect(mockGoBack).toHaveBeenCalledTimes(1)
  })
})

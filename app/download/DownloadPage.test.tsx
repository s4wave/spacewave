import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DownloadPage } from './DownloadPage.js'

vi.mock('@s4wave/web/ui/shooting-stars.js', () => ({
  ShootingStars: () => <div data-testid="shooting-stars" />,
}))

vi.mock('@s4wave/app/landing/LegalFooter.js', () => ({
  LegalFooter: () => <div data-testid="legal-footer" />,
}))

vi.mock('@s4wave/app/landing/useLandingBackNavigation.js', () => ({
  useLandingBackNavigation: () => vi.fn(),
}))

function setUserAgent(userAgent: string) {
  Object.defineProperty(window.navigator, 'userAgent', {
    value: userAgent,
    configurable: true,
  })
}

describe('DownloadPage', () => {
  beforeEach(() => {
    cleanup()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders sections for macOS, Windows, and Linux', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    expect(
      screen.getByRole('heading', { level: 2, name: 'macOS' }),
    ).toBeTruthy()
    expect(
      screen.getByRole('heading', { level: 2, name: 'Windows' }),
    ).toBeTruthy()
    expect(
      screen.getByRole('heading', { level: 2, name: 'Linux' }),
    ).toBeTruthy()
  })

  it('reflects a detected macOS platform in the installer primary CTA', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    const ctas = screen.getAllByRole('link', { name: /Download for macOS/ })
    const installerCta = ctas.find((c) =>
      c.getAttribute('href')?.includes('spacewave-macos-arm64.dmg'),
    )
    expect(installerCta).toBeTruthy()
  })

  it('reflects a detected Windows platform in the installer primary CTA', () => {
    setUserAgent('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36')
    render(<DownloadPage />)

    const ctas = screen.getAllByRole('link', { name: /Download for Windows/ })
    const installerCta = ctas.find(
      (c) =>
        c.getAttribute('href')?.includes('spacewave-windows-amd64.zip') &&
        !c.getAttribute('href')?.includes('cli'),
    )
    expect(installerCta).toBeTruthy()
  })

  it('renders the Spacewave CLI section with a #cli anchor', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    const { container } = render(<DownloadPage />)

    const cliSection = container.querySelector('section#cli')
    expect(cliSection).toBeTruthy()
    expect(
      screen.getByRole('heading', { level: 2, name: 'Spacewave CLI' }),
    ).toBeTruthy()
  })

  it('reflects a detected macOS platform in the CLI primary CTA', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    const ctas = screen.getAllByRole('link', { name: /Download for macOS/ })
    const cliCta = ctas.find((c) =>
      c.getAttribute('href')?.includes('spacewave-cli-macos-arm64.zip'),
    )
    expect(cliCta).toBeTruthy()
  })

  it('renders a macOS zip-extract instruction in the CLI section', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    expect(
      screen.getByText(/macOS.*ships as a signed and notarized zip/i),
    ).toBeTruthy()
  })

  it('renders a Windows zip-extract instruction in the CLI section', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    expect(screen.getByText(/Windows.*ships as a portable zip/i)).toBeTruthy()
  })

  it('renders a curl one-line install snippet for Unix CLI targets', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    expect(
      screen.getAllByText(/curl -fsSL .*spacewave-cli-/i).length,
    ).toBeGreaterThan(0)
  })

  it('shows the Windows bypass notice because the manifest has unsigned entries', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    expect(
      screen.getByRole('button', { name: /How to run on Windows/ }),
    ).toBeTruthy()
  })

  it('falls back to the pick-a-build message when detection misses', () => {
    setUserAgent('UnknownAgent/1.0')
    render(<DownloadPage />)

    // Installer and CLI sections each fall back independently.
    expect(screen.getAllByText('Pick a build below.').length).toBe(2)
  })

  it('surfaces the interim unsigned caption under the Windows heading', () => {
    setUserAgent('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)')
    render(<DownloadPage />)

    expect(screen.getByText('Interim unsigned build')).toBeTruthy()
  })
})

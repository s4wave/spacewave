import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ChangelogReleaseDownloads } from './ChangelogReleaseDownloads.js'

vi.mock('@s4wave/web/platform/detect-platform.js', () => ({
  detectPlatform: () => ({ os: 'macos', arch: 'arm64' }),
}))

describe('ChangelogReleaseDownloads', () => {
  beforeEach(() => cleanup())
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('defaults to the latest release artifacts and highlights the detected platform', () => {
    render(<ChangelogReleaseDownloads version="0.53.1" />)

    const primary = screen.getByRole('link', {
      name: /Download for macOS \(Apple Silicon\)/,
    })
    expect(primary.getAttribute('href')).toBe(
      'https://github.com/s4wave/spacewave/releases/latest/download/spacewave-macos-arm64.dmg',
    )
  })

  it('switches every link to the exact version when the checkbox is ticked', () => {
    render(<ChangelogReleaseDownloads version="0.53.1" />)

    fireEvent.click(
      screen.getByLabelText(/Download this exact version \(v0\.53\.1\)/),
    )

    const primary = screen.getByRole('link', {
      name: /Download for macOS \(Apple Silicon\)/,
    })
    expect(primary.getAttribute('href')).toBe(
      'https://github.com/s4wave/spacewave/releases/download/v0.53.1/spacewave-macos-arm64.dmg',
    )

    const archTile = screen.getByText('spacewave-linux-amd64.AppImage')
    const tileLink = archTile.closest('a')
    expect(tileLink?.getAttribute('href')).toBe(
      'https://github.com/s4wave/spacewave/releases/download/v0.53.1/spacewave-linux-amd64.AppImage',
    )
  })
})

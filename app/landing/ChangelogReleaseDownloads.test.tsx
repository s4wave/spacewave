import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ChangelogReleaseDownloads } from './ChangelogReleaseDownloads.js'

const platform = vi.hoisted(() => ({
  detected: { os: 'macos', arch: 'arm64' } as {
    os: 'macos'
    arch: 'arm64'
  } | null,
}))

vi.mock('@s4wave/web/platform/detect-platform.js', () => ({
  detectPlatform: () => platform.detected,
}))

describe('ChangelogReleaseDownloads', () => {
  beforeEach(() => {
    cleanup()
    platform.detected = { os: 'macos', arch: 'arm64' }
  })
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('defaults to the latest release and labels the detected artifact', () => {
    render(<ChangelogReleaseDownloads version="0.53.1" />)

    expect(
      screen.getByRole('radio', { name: /Latest release/ }),
    ).toHaveProperty('checked', true)
    const primary = screen.getByRole('link', {
      name: 'Download Latest release for macOS (Apple Silicon) · dmg',
    })
    expect(primary.getAttribute('href')).toBe(
      'https://github.com/s4wave/spacewave/releases/latest/download/spacewave-macos-arm64.dmg',
    )
    expect(primary.getAttribute('download')).not.toBeNull()
    expect(primary.getAttribute('target')).toBe('_blank')
    expect(primary.getAttribute('rel')).toBe('noopener noreferrer')
  })

  it('switches every link to the exact release selected by its radio', () => {
    render(<ChangelogReleaseDownloads version="0.53.1" />)

    const exactRelease = screen.getByRole('radio', {
      name: /This release · v0\.53\.1/,
    })
    exactRelease.focus()
    expect(document.activeElement).toBe(exactRelease)
    fireEvent.click(exactRelease)

    expect(
      screen.getByRole('link', {
        name: 'Download v0.53.1 for macOS (Apple Silicon) · dmg',
      }),
    ).toHaveProperty(
      'href',
      'https://github.com/s4wave/spacewave/releases/download/v0.53.1/spacewave-macos-arm64.dmg',
    )

    const archTile = screen.getByText('spacewave-linux-amd64.AppImage')
    expect(archTile.closest('a')?.getAttribute('href')).toBe(
      'https://github.com/s4wave/spacewave/releases/download/v0.53.1/spacewave-linux-amd64.AppImage',
    )
  })

  it('offers manual builds when platform detection is unsupported', () => {
    platform.detected = null
    render(<ChangelogReleaseDownloads version="0.53.1" />)

    expect(
      screen.queryByRole('link', { name: /Download Latest release for/ }),
    ).toBeNull()
    expect(
      screen.getByText(/could not detect a supported build for this device/i),
    ).toBeTruthy()
    expect(screen.getByText('spacewave-linux-amd64.AppImage')).toBeTruthy()
  })

  it.each(['', ' 0.53.1', '0.53.1 ', 'v0.53.1', 'garbage'])(
    'disables exact-release selection for invalid version %j',
    (version) => {
      render(<ChangelogReleaseDownloads version={version} />)

      expect(
        screen.getByRole('radio', { name: /This release/ }),
      ).toHaveProperty('disabled', true)
      expect(screen.getByText('Selected: Latest release')).toBeTruthy()
      expect(
        document.querySelector('a[href*="/releases/download/v"]'),
      ).toBeNull()
    },
  )
})

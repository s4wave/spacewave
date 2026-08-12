import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ChangelogReleasePage } from './ChangelogReleasePage.js'

vi.mock('@s4wave/web/platform/detect-platform.js', () => ({
  detectPlatform: () => ({ os: 'macos', arch: 'arm64' }),
}))

vi.mock('./useLandingBackNavigation.js', () => ({
  useLandingBackNavigation: () => vi.fn(),
}))

vi.mock('./LegalFooter.js', () => ({
  LegalFooter: () => <div>footer</div>,
}))

describe('ChangelogReleasePage', () => {
  beforeEach(() => cleanup())
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the release hero, notes, and desktop download section', () => {
    render(
      <ChangelogReleasePage
        release={{
          version: '0.53.1',
          date: '2026-07-05',
          summary: 'Release summary.',
          features: [{ description: 'A shiny new feature' }],
        }}
      />,
    )

    expect(
      screen.getByRole('heading', { level: 1, name: 'Spacewave v0.53.1' }),
    ).toBeTruthy()
    expect(screen.getByText('A shiny new feature')).toBeTruthy()
    expect(
      screen.getByRole('heading', { name: 'Download the desktop app' }),
    ).toBeTruthy()
    expect(
      screen.getByRole('radio', { name: /This release · v0\.53\.1/ }),
    ).toBeTruthy()
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'

import { buildShellPopoutUrl, openShellTabInNewTab } from './shell-popout.js'

describe('shell-popout', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('buildShellPopoutUrl normalizes shell paths into hash URLs', () => {
    expect(
      buildShellPopoutUrl('u/7/docs', {
        origin: 'https://spacewave.app',
        pathname: '/app',
      }),
    ).toBe('https://spacewave.app/app#/u/7/docs')

    expect(
      buildShellPopoutUrl('#/docs', {
        origin: 'https://spacewave.app',
        pathname: '/app',
      }),
    ).toBe('https://spacewave.app/app#/docs')
  })

  it('openShellTabInNewTab requests a new tab without popup window features', () => {
    const openSpy = vi
      .spyOn(window, 'open')
      .mockImplementation(() => null as WindowProxy | null)

    openShellTabInNewTab('/docs')

    expect(openSpy).toHaveBeenCalledWith(
      `${window.location.origin}${window.location.pathname}#/docs`,
      '_blank',
      'noopener,noreferrer',
    )
  })
})

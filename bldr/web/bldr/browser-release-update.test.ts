import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  initBrowserReleaseAutoReload,
  shouldReloadForPromotedGeneration,
} from './browser-release-update.js'

describe('browser release update reload policy', () => {
  it('does not reload when the promoted generation matches the active shell', () => {
    expect(
      shouldReloadForPromotedGeneration('deadbeefcafebabe', 'deadbeefcafebabe'),
    ).toBe(false)
  })

  it('reloads when the promoted generation changes', () => {
    expect(
      shouldReloadForPromotedGeneration('deadbeefcafebabe', 'feedfacecafed00d'),
    ).toBe(true)
  })

  it('ignores empty promotion messages', () => {
    expect(
      shouldReloadForPromotedGeneration('deadbeefcafebabe', undefined),
    ).toBe(false)
  })
})

describe('browser release lifecycle sync probes', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('posts bldrSyncManifest when the page becomes visible, focused, or online', () => {
    const postMessage = vi.fn()
    vi.stubGlobal('navigator', {
      serviceWorker: {
        addEventListener: vi.fn(),
        controller: {
          postMessage,
        },
      },
    })
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    })

    initBrowserReleaseAutoReload()
    document.dispatchEvent(new Event('visibilitychange'))
    window.dispatchEvent(new Event('focus'))
    window.dispatchEvent(new Event('online'))

    expect(postMessage).toHaveBeenCalledTimes(3)
    expect(postMessage).toHaveBeenNthCalledWith(1, {
      bldrSyncManifest: true,
    })
    expect(postMessage).toHaveBeenNthCalledWith(2, {
      bldrSyncManifest: true,
    })
    expect(postMessage).toHaveBeenNthCalledWith(3, {
      bldrSyncManifest: true,
    })
  })
})

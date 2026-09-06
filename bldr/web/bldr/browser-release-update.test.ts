import { afterEach, describe, expect, it, vi } from 'vitest'

import { initBrowserReleaseUpdates } from './browser-release-update.js'

describe('browser release lifecycle sync probes', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('does not install an automatic reload handler for release promotion', () => {
    const addEventListener = vi.fn()
    vi.stubGlobal('navigator', { serviceWorker: { addEventListener } })
    vi.spyOn(document, 'addEventListener').mockImplementation(() => {})
    vi.spyOn(window, 'addEventListener').mockImplementation(() => {})
    initBrowserReleaseUpdates()
    expect(addEventListener).not.toHaveBeenCalled()
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

    initBrowserReleaseUpdates()
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

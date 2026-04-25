import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  getBrowserCheckoutResultBaseUrl,
  getCheckoutResultBaseUrl,
} from './checkout-url.js'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('getCheckoutResultBaseUrl', () => {
  it('prefers accountBaseUrl over publicBaseUrl', () => {
    expect(
      getCheckoutResultBaseUrl({
        accountBaseUrl: 'https://account.spacewave.example/',
        publicBaseUrl: 'https://spacewave.example/',
      }),
    ).toBe('https://account.spacewave.example')
  })

  it('falls back to publicBaseUrl when accountBaseUrl is unavailable', () => {
    expect(
      getCheckoutResultBaseUrl({
        publicBaseUrl: 'https://spacewave.example/',
      }),
    ).toBe('https://spacewave.example')
  })

  it('returns empty string when cloud provider config is unavailable', () => {
    expect(getCheckoutResultBaseUrl(null)).toBe('')
  })

  it('falls back to browser origin when config is unavailable', () => {
    vi.stubGlobal('window', {
      location: {
        origin: 'https://staging.spacewave.app/',
      },
    })

    expect(getBrowserCheckoutResultBaseUrl(null)).toBe(
      'https://staging.spacewave.app',
    )
  })
})

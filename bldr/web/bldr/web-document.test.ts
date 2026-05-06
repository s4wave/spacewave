import { afterEach, describe, expect, it, vi } from 'vitest'

import { registerUpdatedServiceWorker } from './web-document.js'

describe('registerUpdatedServiceWorker', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('registers the manifest service worker URL when it differs', async () => {
    const register = vi.fn().mockResolvedValue({})
    const registration = {
      scope: 'https://example.test/',
    } as ServiceWorkerRegistration

    await registerUpdatedServiceWorker(
      '/sw-a.mjs',
      registration,
      register,
      '/sw-b.mjs',
    )

    expect(register).toHaveBeenCalledWith(
      new URL('/sw-b.mjs', location.href).toString(),
      {
        scope: registration.scope,
      },
    )
  })

  it('does not re-register when the URLs match', async () => {
    const register = vi.fn().mockResolvedValue({})

    const result = await registerUpdatedServiceWorker(
      '/sw-a.mjs',
      undefined,
      register,
      '/sw-a.mjs',
    )

    expect(result).toBeNull()
    expect(register).not.toHaveBeenCalled()
  })
})

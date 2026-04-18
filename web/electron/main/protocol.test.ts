import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('electron', () => ({
  default: {
    app: {
      getAppPath() {
        return '/app'
      },
    },
    net: {
      fetch: vi.fn(),
    },
    protocol: {
      registerSchemesAsPrivileged: vi.fn(),
    },
  },
}))

describe('electron protocol', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it('extracts the WebDocument id from the referrer query string', async () => {
    const { extractWebDocumentClientId } = await import('./protocol.js')
    const req = buildRequestLike(
      'app://index.html/b/pa/plugin/v/b/fe/app/App.mjs',
      'app://index.html?webDocumentId=electron-init',
    )

    expect(extractWebDocumentClientId(req)).toBe('electron-init')
  })

  it('forwards ServiceWorker-owned requests with the extracted client id', async () => {
    const { appRequestHandler } = await import('./protocol.js')
    const swFetch = vi.fn().mockResolvedValue(new Response('ok'))
    const req = buildRequestLike(
      'app://index.html/b/pa/plugin/v/b/fe/app/App.mjs',
      'app://index.html?webDocumentId=electron-init',
    )

    const resp = await appRequestHandler(swFetch, req)

    expect(await resp.text()).toBe('ok')
    expect(swFetch).toHaveBeenCalledWith(req, 'electron-init')
  })

  it('falls back to the request url when the referrer is absent', async () => {
    const { extractWebDocumentClientId } = await import('./protocol.js')
    const req = buildRequestLike(
      'app://index.html/b/pd/plugin/plugin.mjs?webDocumentId=popout-1',
    )

    expect(extractWebDocumentClientId(req)).toBe('popout-1')
  })
})

function buildRequestLike(url: string, referrer = ''): GlobalRequest {
  return {
    url,
    referrer,
    headers: {
      get(name: string) {
        if (name.toLowerCase() === 'referer') {
          return referrer || null
        }
        return null
      },
    },
  } as GlobalRequest
}

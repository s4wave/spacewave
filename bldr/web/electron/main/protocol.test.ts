import { beforeEach, describe, expect, it, vi } from 'vitest'

const electronMock = vi.hoisted(() => ({
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
}))

vi.mock('electron', () => ({
  default: electronMock,
}))

describe('electron protocol', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
  })

  it('registers app scheme as fetch-capable without ServiceWorker privilege', async () => {
    // Dynamic import intentionally re-evaluates the module-level Electron scheme
    // registration side effect after vi.resetModules().
    const { APP_SCHEME } = await import('./protocol.js')

    expect(
      electronMock.protocol.registerSchemesAsPrivileged,
    ).toHaveBeenCalledWith([
      {
        scheme: APP_SCHEME,
        privileges: {
          standard: true,
          secure: true,
          bypassCSP: true,
          supportFetchAPI: true,
          corsEnabled: true,
          stream: true,
        },
      },
    ])
    const [registrations] =
      electronMock.protocol.registerSchemesAsPrivileged.mock.calls[0]
    expect(registrations[0]?.privileges).not.toHaveProperty(
      'allowServiceWorkers',
    )
  })

  it('extracts the WebDocument id from the referrer query string', async () => {
    // Dynamic import re-evaluates Electron scheme registration after vi.resetModules().
    const { extractWebDocumentClientId } = await import('./protocol.js')
    const req = buildRequestLike(
      'app://index.html/b/pa/plugin/v/b/fe/app/App.mjs',
      'app://index.html?webDocumentId=electron-init',
    )

    expect(extractWebDocumentClientId(req)).toBe('electron-init')
  })

  it('forwards plugin asset requests with the extracted client id and live headers', async () => {
    // Dynamic import re-evaluates Electron scheme registration after vi.resetModules().
    const { appRequestHandler } = await import('./protocol.js')
    const swFetch = vi.fn().mockResolvedValue(
      new Response('ok', {
        headers: { 'content-type': 'application/javascript' },
      }),
    )
    const req = buildRequestLike(
      'app://index.html/b/pa/plugin/v/b/fe/app/App.mjs',
      'app://index.html?webDocumentId=electron-init',
    )

    const resp = await appRequestHandler(swFetch, req)

    expect(await resp.text()).toBe('ok')
    expect(resp.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(resp.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe('live')
    expect(swFetch).toHaveBeenCalledWith(req, 'electron-init')
  })

  it('leaves non-plugin Bldr runtime responses unclassified', async () => {
    // Dynamic import re-evaluates Electron scheme registration after vi.resetModules().
    const { appRequestHandler } = await import('./protocol.js')
    const swFetch = vi.fn().mockResolvedValue(new Response('ok'))
    const req = buildRequestLike(
      'app://index.html/b/pkg/@aptre/bldr/index.js',
      'app://index.html?webDocumentId=electron-init',
    )

    const resp = await appRequestHandler(swFetch, req)

    expect(await resp.text()).toBe('ok')
    expect(resp.headers.get('X-Bldr-Fetch-Source')).toBeNull()
    expect(resp.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBeNull()
    expect(swFetch).toHaveBeenCalledWith(req, 'electron-init')
  })

  it('falls back to the request url when the referrer is absent', async () => {
    // Dynamic import re-evaluates Electron scheme registration after vi.resetModules().
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

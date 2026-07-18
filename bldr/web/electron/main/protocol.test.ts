import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import path from 'path'

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

  afterEach(() => {
    vi.doUnmock('path')
  })

  it('registers app scheme as fetch-capable without ServiceWorker privilege', async () => {
    const { APP_SCHEME } = await importProtocolWithFreshModuleState()

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
    const { extractWebDocumentClientId } =
      await importProtocolWithFreshModuleState()
    const req = buildRequestLike(
      'app://index.html/b/pa/plugin/v/b/fe/app/App.mjs',
      'app://index.html?webDocumentId=electron-init',
    )

    expect(extractWebDocumentClientId(req)).toBe('electron-init')
  })

  it('forwards plugin asset requests with the extracted client id, live headers, and cross-origin isolation headers', async () => {
    const { appRequestHandler } = await importProtocolWithFreshModuleState()
    const swFetch = vi.fn().mockResolvedValue(
      new Response('sw body', {
        status: 203,
        statusText: 'Non-Authoritative Information',
        headers: {
          'Content-Type': 'application/javascript',
          'X-Forwarded-By': 'service-worker',
        },
      }),
    )
    const req = buildRequestLike(
      'app://index.html/b/pa/plugin/v/b/fe/app/App.mjs',
      'app://index.html?webDocumentId=electron-init',
    )

    const resp = await appRequestHandler(swFetch, req)

    expect(resp.status).toBe(203)
    expect(resp.statusText).toBe('Non-Authoritative Information')
    expect(resp.headers.get('Content-Type')).toBe('application/javascript')
    expect(resp.headers.get('X-Forwarded-By')).toBe('service-worker')
    expect(resp.headers.get('X-Bldr-Fetch-Source')).toBe('plugin-assets')
    expect(resp.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBe('live')
    expectCrossOriginIsolationHeaders(resp)
    expect(await resp.text()).toBe('sw body')
    expect(swFetch).toHaveBeenCalledWith(req, 'electron-init')
  })

  it('keeps Bldr request paths slash-separated on Windows', async () => {
    vi.doMock('path', () => ({ default: path.win32 }))
    const { appRequestHandler } = await importProtocolWithFreshModuleState()
    const swFetch = vi.fn().mockResolvedValue(new Response('windows-ok'))
    const req = buildRequestLike(
      'app://index.html/b/pa/plugin/v/b/fe/app/App.mjs',
      'app://index.html?webDocumentId=electron-init',
    )

    const resp = await appRequestHandler(swFetch, req)

    expect(await resp.text()).toBe('windows-ok')
    expect(swFetch).toHaveBeenCalledWith(req, 'electron-init')
    expect(electronMock.net.fetch).not.toHaveBeenCalled()
  })

  it('leaves non-plugin Bldr runtime responses unclassified while adding cross-origin isolation headers', async () => {
    const { appRequestHandler } = await importProtocolWithFreshModuleState()
    const swFetch = vi.fn().mockResolvedValue(new Response('ok'))
    const req = buildRequestLike(
      'app://index.html/b/pkg/@aptre/bldr/index.js',
      'app://index.html?webDocumentId=electron-init',
    )

    const resp = await appRequestHandler(swFetch, req)

    expect(await resp.text()).toBe('ok')
    expect(resp.headers.get('X-Bldr-Fetch-Source')).toBeNull()
    expect(resp.headers.get('X-Bldr-Plugin-Asset-Fetch-Result')).toBeNull()
    expectCrossOriginIsolationHeaders(resp)
    expect(swFetch).toHaveBeenCalledWith(req, 'electron-init')
  })

  it('adds cross-origin isolation headers to app file responses without changing the fetched response payload', async () => {
    const { appRequestHandler } = await importProtocolWithFreshModuleState()
    electronMock.net.fetch.mockResolvedValueOnce(
      new Response('asset body', {
        status: 202,
        statusText: 'Accepted',
        headers: {
          'Content-Type': 'application/javascript',
          'Cross-Origin-Embedder-Policy': 'unsafe-none',
          'X-Asset-Source': 'electron-net-fetch',
        },
      }),
    )

    const resp = await appRequestHandler(
      vi.fn(),
      buildRequestLike('app://index.html/assets/main.js'),
    )

    expect(resp.status).toBe(202)
    expect(resp.statusText).toBe('Accepted')
    expect(resp.headers.get('Content-Type')).toBe('application/javascript')
    expect(resp.headers.get('X-Asset-Source')).toBe('electron-net-fetch')
    expectCrossOriginIsolationHeaders(resp)
    expect(await resp.text()).toBe('asset body')
  })

  it('falls back to the request url when the referrer is absent', async () => {
    const { extractWebDocumentClientId } =
      await importProtocolWithFreshModuleState()
    const req = buildRequestLike(
      'app://index.html/b/pd/plugin/plugin.mjs?webDocumentId=popout-1',
    )

    expect(extractWebDocumentClientId(req)).toBe('popout-1')
  })
})

function importProtocolWithFreshModuleState() {
  return import('./protocol.js')
}

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

function expectCrossOriginIsolationHeaders(resp: GlobalResponse) {
  expect(resp.headers.get('Cross-Origin-Opener-Policy')).toBe('same-origin')
  expect(resp.headers.get('Cross-Origin-Embedder-Policy')).toBe(
    'credentialless',
  )
  expect(resp.headers.get('Cross-Origin-Resource-Policy')).toBe('same-origin')
}

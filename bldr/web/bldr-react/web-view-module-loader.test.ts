import { describe, expect, it, vi } from 'vitest'

import {
  fetchWebViewRootAssetResult,
  isWebViewRootPluginAssetPath,
  loadWebViewScriptModule,
  WebViewRootAssetLoadError,
  webViewRootAssetStatusEvent,
} from './web-view-module-loader.js'

function pluginAssetResponse(
  status: number,
  body: string,
  headers: Record<string, string> = {},
): Response {
  return new Response(body, {
    status,
    headers,
  })
}

describe('WebView root module loader', () => {
  it('skips the root asset probe for non-plugin asset paths', async () => {
    const fetchRootAsset = vi.fn<typeof fetch>()
    const importModule = vi.fn(async () => ({ default: 'module' }))

    await expect(
      loadWebViewScriptModule('/component.js', {
        fetchRootAsset,
        importModule,
      }),
    ).resolves.toEqual({ default: 'module' })

    expect(fetchRootAsset).not.toHaveBeenCalled()
    expect(importModule).toHaveBeenCalledWith('/component.js')
  })

  it('reports a missing root plugin asset before module import', async () => {
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(404, 'missing plugin asset body', {
        'content-type': 'text/plain',
        'X-Bldr-Fetch-Source': 'plugin-assets',
        'X-Bldr-Runtime-Fetch-Error': 'plugin-asset-missing',
        'X-Bldr-Plugin-Asset-Fetch-Result': 'missing',
      }),
    )
    const importModule = vi.fn(async () => ({ default: 'module' }))

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App-old.mjs', {
        fetchRootAsset,
        importModule,
      }),
    ).rejects.toMatchObject({
      name: 'WebViewRootAssetLoadError',
      rootAsset: {
        scriptPath: '/b/pa/spacewave-app/v/app/App-old.mjs',
        status: 404,
        ok: false,
        fetchSource: 'plugin-assets',
        runtimeError: 'plugin-asset-missing',
        pluginAssetResult: 'missing',
        classification: 'missing',
        contentType: 'text/plain',
        bodyPrefix: 'missing plugin asset body',
      },
    })

    expect(fetchRootAsset).toHaveBeenCalledWith(
      '/b/pa/spacewave-app/v/app/App-old.mjs',
      { cache: 'no-store' },
    )
    expect(importModule).not.toHaveBeenCalled()
  })

  it('preserves nested import and module evaluation failures after a live root asset', async () => {
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(200, 'export default function App() {}', {
        'content-type': 'text/javascript',
        'X-Bldr-Fetch-Source': 'plugin-assets',
        'X-Bldr-Plugin-Asset-Fetch-Result': 'live',
      }),
    )
    const nestedFailure = new Error(
      'Failed to fetch dynamically imported module: /b/pa/spacewave-app/v/chunk-missing.mjs',
    )
    const importModule = vi.fn(async () => {
      throw nestedFailure
    })

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App-live.mjs', {
        fetchRootAsset,
        importModule,
      }),
    ).rejects.toBe(nestedFailure)

    expect(fetchRootAsset).toHaveBeenCalledOnce()
    expect(importModule).toHaveBeenCalledWith(
      '/b/pa/spacewave-app/v/app/App-live.mjs',
    )
  })

  it('imports the module after a live root plugin asset result', async () => {
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(200, 'export default function App() {}', {
        'content-type': 'text/javascript',
        'X-Bldr-Fetch-Source': 'plugin-assets',
        'X-Bldr-Plugin-Asset-Fetch-Result': 'live',
      }),
    )
    const importModule = vi.fn(async () => ({ default: 'component' }))

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App-live.mjs', {
        fetchRootAsset,
        importModule,
      }),
    ).resolves.toEqual({ default: 'component' })

    expect(fetchRootAsset).toHaveBeenCalledOnce()
    expect(importModule).toHaveBeenCalledWith(
      '/b/pa/spacewave-app/v/app/App-live.mjs',
    )
  })

  it('classifies plugin asset requests that bypass Bldr response headers', async () => {
    await expect(
      fetchWebViewRootAssetResult(
        '/b/pa/spacewave-app/v/app/App-bypass.mjs',
        vi.fn(async () =>
          pluginAssetResponse(404, 'ordinary 404 without bldr headers', {
            'content-type': 'text/plain',
          }),
        ),
      ),
    ).resolves.toMatchObject({
      scriptPath: '/b/pa/spacewave-app/v/app/App-bypass.mjs',
      status: 404,
      ok: false,
      classification: 'bypass',
      bodyPrefix: 'ordinary 404 without bldr headers',
    })
  })

  it('records the latest typed root asset result for status readers', async () => {
    const events: unknown[] = []
    const listener = (event: Event) => {
      events.push((event as CustomEvent).detail)
    }
    globalThis.addEventListener(webViewRootAssetStatusEvent, listener)
    try {
      const result = await fetchWebViewRootAssetResult(
        '/b/pa/spacewave-app/v/app/App-live.mjs',
        vi.fn(async () =>
          pluginAssetResponse(200, 'export default function App() {}', {
            'content-type': 'text/javascript',
            'X-Bldr-Fetch-Source': 'plugin-assets',
            'X-Bldr-Plugin-Asset-Fetch-Result': 'live',
          }),
        ),
      )

      expect(result).toMatchObject({
        scriptPath: '/b/pa/spacewave-app/v/app/App-live.mjs',
        status: 200,
        ok: true,
        classification: 'live',
      })
      expect(globalThis.__bldrWebViewRootAssetStatus).toMatchObject(result)
      expect(events).toHaveLength(1)
      expect(events[0]).toMatchObject(result)
    } finally {
      globalThis.removeEventListener(webViewRootAssetStatusEvent, listener)
      globalThis.__bldrWebViewRootAssetStatus = undefined
    }
  })

  it('recognizes absolute root plugin asset URLs', () => {
    expect(
      isWebViewRootPluginAssetPath(
        'app://index.html/b/pa/spacewave-app/v/app/App.mjs',
      ),
    ).toBe(true)
    expect(isWebViewRootPluginAssetPath('/b/other/path.mjs')).toBe(false)
  })

  it('exposes the typed root asset error class for boundaries', () => {
    const error = new WebViewRootAssetLoadError({
      scriptPath: '/b/pa/spacewave-app/v/app/App-old.mjs',
      status: 410,
      ok: false,
      classification: 'generation-closed',
    })

    expect(error.message).toContain('generation-closed')
    expect(error.rootAsset.status).toBe(410)
  })
})

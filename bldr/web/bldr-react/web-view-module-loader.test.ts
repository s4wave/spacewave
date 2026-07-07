import { describe, expect, it, vi } from 'vitest'

import {
  fetchWebViewRootAssetResult,
  isWebViewRootPluginAssetPath,
  loadWebViewScriptModule,
  WebViewRootAssetLoadError,
  webViewModuleImportErrorEvent,
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

function deferred<T>(): {
  promise: Promise<T>
  resolve: (value: T | PromiseLike<T>) => void
  reject: (reason?: unknown) => void
} {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
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
    const events: unknown[] = []
    const listener = (event: Event) => {
      if (event instanceof CustomEvent) {
        events.push(event.detail)
      }
    }
    globalThis.addEventListener(webViewModuleImportErrorEvent, listener)
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
    nestedFailure.name = 'TypeError'
    const importModule = vi.fn(async () => {
      throw nestedFailure
    })

    try {
      await expect(
        loadWebViewScriptModule(
          '/b/pa/spacewave-app/v/app/App-nested-failure.mjs',
          {
            fetchRootAsset,
            importModule,
          },
        ),
      ).rejects.toBe(nestedFailure)

      expect(fetchRootAsset).toHaveBeenCalledOnce()
      expect(importModule).toHaveBeenCalledWith(
        '/b/pa/spacewave-app/v/app/App-nested-failure.mjs',
      )
      expect(globalThis.__bldrWebViewModuleImportError).toMatchObject({
        scriptPath: '/b/pa/spacewave-app/v/app/App-nested-failure.mjs',
        name: 'TypeError',
        message:
          'Failed to fetch dynamically imported module: /b/pa/spacewave-app/v/chunk-missing.mjs',
        rootAsset: {
          scriptPath: '/b/pa/spacewave-app/v/app/App-nested-failure.mjs',
          status: 200,
          ok: true,
          classification: 'live',
        },
      })
      expect(events).toHaveLength(1)
      expect(events[0]).toMatchObject(
        globalThis.__bldrWebViewModuleImportError ?? {},
      )
    } finally {
      globalThis.removeEventListener(webViewModuleImportErrorEvent, listener)
      globalThis.__bldrWebViewModuleImportError = undefined
      globalThis.__bldrWebViewRootAssetStatus = undefined
    }
  })

  it('adds a retry nonce after module import failure and clears it after success', async () => {
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(200, 'export default function App() {}', {
        'content-type': 'text/javascript',
        'X-Bldr-Fetch-Source': 'plugin-assets',
        'X-Bldr-Plugin-Asset-Fetch-Result': 'live',
      }),
    )
    const importFailure = new TypeError(
      'Failed to fetch dynamically imported module: /b/pa/spacewave-app/v/app/App-retry.mjs',
    )
    const importModule = vi
      .fn()
      .mockRejectedValueOnce(importFailure)
      .mockResolvedValueOnce({ default: 'component' })
      .mockResolvedValueOnce({ default: 'component' })

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App-retry.mjs', {
        fetchRootAsset,
        importModule,
      }),
    ).rejects.toBe(importFailure)

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App-retry.mjs', {
        fetchRootAsset,
        importModule,
      }),
    ).resolves.toEqual({ default: 'component' })

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App-retry.mjs', {
        fetchRootAsset,
        importModule,
      }),
    ).resolves.toEqual({ default: 'component' })

    expect(importModule).toHaveBeenNthCalledWith(
      1,
      '/b/pa/spacewave-app/v/app/App-retry.mjs',
    )
    expect(importModule).toHaveBeenNthCalledWith(
      2,
      '/b/pa/spacewave-app/v/app/App-retry.mjs?bldr_retry=1',
    )
    expect(importModule).toHaveBeenNthCalledWith(
      3,
      '/b/pa/spacewave-app/v/app/App-retry.mjs',
    )
  })

  it('coalesces concurrent root plugin module loads for the same retry nonce', async () => {
    const scriptPath = '/b/pa/spacewave-app/v/app/App-coalesce-success.mjs'
    const rootAsset = deferred<Response>()
    const importedModule = { default: 'component' }
    const moduleLoad = deferred<typeof importedModule>()
    const importStarted = deferred<void>()
    const fetchRootAsset = vi.fn(async () => await rootAsset.promise)
    const importModule = vi.fn(() => {
      importStarted.resolve()
      return moduleLoad.promise
    })

    const firstLoad = loadWebViewScriptModule(scriptPath, {
      fetchRootAsset,
      importModule,
    })
    const secondLoad = loadWebViewScriptModule(scriptPath, {
      fetchRootAsset,
      importModule,
    })

    expect(fetchRootAsset).toHaveBeenCalledOnce()
    expect(importModule).not.toHaveBeenCalled()

    rootAsset.resolve(
      pluginAssetResponse(200, 'export default function App() {}', {
        'content-type': 'text/javascript',
        'X-Bldr-Fetch-Source': 'plugin-assets',
        'X-Bldr-Plugin-Asset-Fetch-Result': 'live',
      }),
    )
    await importStarted.promise

    expect(importModule).toHaveBeenCalledOnce()
    expect(importModule).toHaveBeenCalledWith(scriptPath)

    moduleLoad.resolve(importedModule)
    const [firstModule, secondModule] = await Promise.all([
      firstLoad,
      secondLoad,
    ])

    expect(firstModule).toBe(importedModule)
    expect(secondModule).toBe(importedModule)
  })

  it('coalesces concurrent import failures and lets the next retry issue a fresh load', async () => {
    const scriptPath = '/b/pa/spacewave-app/v/app/App-coalesce-retry.mjs'
    const importFailure = new TypeError(
      'Failed to fetch dynamically imported module: /b/pa/spacewave-app/v/app/App-coalesce-retry.mjs',
    )
    const failedImport = deferred<never>()
    const firstImportStarted = deferred<void>()
    const retryModule = { default: 'retry component' }
    let importCalls = 0
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(200, 'export default function App() {}', {
        'content-type': 'text/javascript',
        'X-Bldr-Fetch-Source': 'plugin-assets',
        'X-Bldr-Plugin-Asset-Fetch-Result': 'live',
      }),
    )
    const importModule = vi.fn(() => {
      importCalls += 1
      if (importCalls === 1) {
        firstImportStarted.resolve()
        return failedImport.promise
      }
      return Promise.resolve(retryModule)
    })

    try {
      const firstLoad = loadWebViewScriptModule(scriptPath, {
        fetchRootAsset,
        importModule,
      })
      const secondLoad = loadWebViewScriptModule(scriptPath, {
        fetchRootAsset,
        importModule,
      })

      await firstImportStarted.promise

      expect(fetchRootAsset).toHaveBeenCalledOnce()
      expect(importModule).toHaveBeenCalledOnce()
      expect(importModule).toHaveBeenNthCalledWith(1, scriptPath)

      failedImport.reject(importFailure)
      const [firstResult, secondResult] = await Promise.allSettled([
        firstLoad,
        secondLoad,
      ])

      expect(firstResult).toEqual({
        status: 'rejected',
        reason: importFailure,
      })
      expect(secondResult).toEqual({
        status: 'rejected',
        reason: importFailure,
      })

      await expect(
        loadWebViewScriptModule(scriptPath, {
          fetchRootAsset,
          importModule,
        }),
      ).resolves.toBe(retryModule)

      expect(fetchRootAsset).toHaveBeenCalledTimes(2)
      expect(importModule).toHaveBeenCalledTimes(2)
      expect(importModule).toHaveBeenNthCalledWith(
        2,
        `${scriptPath}?bldr_retry=1`,
      )
    } finally {
      globalThis.__bldrWebViewModuleImportError = undefined
      globalThis.__bldrWebViewRootAssetStatus = undefined
    }
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

  it('rejects bypassed root plugin asset responses before module import', async () => {
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(200, '<!doctype html><title>loading</title>', {
        'content-type': 'text/html',
      }),
    )
    const importModule = vi.fn(async () => ({ default: 'module' }))

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App-bypass.mjs', {
        fetchRootAsset,
        importModule,
      }),
    ).rejects.toMatchObject({
      name: 'WebViewRootAssetLoadError',
      rootAsset: {
        scriptPath: '/b/pa/spacewave-app/v/app/App-bypass.mjs',
        status: 200,
        ok: true,
        classification: 'bypass',
        contentType: 'text/html',
      },
    })

    expect(fetchRootAsset).toHaveBeenCalledOnce()
    expect(importModule).not.toHaveBeenCalled()
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

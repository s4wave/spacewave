import { describe, expect, it, vi } from 'vitest'

import {
  fetchWebViewRootAssetResult,
  isWebViewRootPluginAssetPath,
  loadWebViewScriptModule,
  reloadForFailedModuleGraph,
  WebViewModuleImportStallError,
  WebViewRootAssetLoadError,
  webViewModuleImportErrorEvent,
  webViewRootAssetStatusEvent,
} from './web-view-module-loader.js'
import { readBootDownloads } from '../bldr/boot-downloads.js'

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

describe('WebView boot deadlines', () => {
  it('preserves the import failure when its diagnostic probe never settles', async () => {
    const fetchRootAsset = vi.fn<typeof fetch>(
      () => new Promise<Response>(() => undefined),
    )
    const importFailure = new Error('module evaluation failed')
    const importModule = vi.fn(async () => {
      throw importFailure
    })

    await expect(
      loadWebViewScriptModule('/b/pa/spacewave-app/v/app/App.mjs', {
        fetchRootAsset,
        importModule,
        // The probe shares what remains of the import deadline; a long import
        // deadline keeps the probe's own 10ms bound in effect on slow hosts.
        fetchDeadlineMillis: 10,
        importDeadlineMillis: 60_000,
      }),
    ).rejects.toBe(importFailure)

    const downloads = readBootDownloads()
    const failed = downloads.find((download) => download.state === 'error')
    expect(failed?.error).toContain('module evaluation failed')
    expect(fetchRootAsset).toHaveBeenCalledOnce()
    expect(fetchRootAsset.mock.calls[0]?.[1]?.signal?.aborted).toBe(true)
    expect(globalThis.__bldrWebViewModuleImportError?.message).toBe(
      importFailure.message,
    )
  })

  it('reports a recoverable stall and keeps the download active', async () => {
    const fetchRootAsset = vi.fn<typeof fetch>()
    const importModule = vi.fn(() => new Promise<unknown>(() => undefined))
    const scriptPath = '/b/pa/spacewave-app/v/app/Stalled.mjs'

    await expect(
      loadWebViewScriptModule(scriptPath, {
        fetchRootAsset,
        importModule,
        importDeadlineMillis: 10,
      }),
    ).rejects.toBeInstanceOf(WebViewModuleImportStallError)

    const download = readBootDownloads().find((d) => d.id === scriptPath)
    expect(download?.state).toBe('active')
    expect(fetchRootAsset).not.toHaveBeenCalled()
    expect(globalThis.__bldrWebViewModuleImportError?.message).toContain(
      'still loading',
    )
  })

  it('retries a stalled import by waiting on the same import', async () => {
    const pending = deferred<unknown>()
    const importModule = vi.fn(() => pending.promise)
    const scriptPath = '/b/pa/spacewave-app/v/app/Slow.mjs'

    await expect(
      loadWebViewScriptModule(scriptPath, {
        importModule,
        importDeadlineMillis: 10,
      }),
    ).rejects.toBeInstanceOf(WebViewModuleImportStallError)

    const retry = loadWebViewScriptModule(scriptPath, {
      importModule,
      importDeadlineMillis: 60_000,
    })
    pending.resolve({ default: 'slow' })
    await expect(retry).resolves.toEqual({ default: 'slow' })
    expect(importModule).toHaveBeenCalledOnce()
    const download = readBootDownloads().find((d) => d.id === scriptPath)
    expect(download?.state).toBe('complete')
  })

  it('starts a fresh import after repeated stalls', async () => {
    const importModule = vi.fn<(path: string) => Promise<unknown>>(
      () => new Promise<unknown>(() => undefined),
    )
    const scriptPath = '/b/pa/spacewave-app/v/app/Wedged.mjs'
    const options = { importModule, importDeadlineMillis: 10 }

    for (let attempt = 0; attempt < 2; attempt++) {
      await expect(
        loadWebViewScriptModule(scriptPath, options),
      ).rejects.toBeInstanceOf(WebViewModuleImportStallError)
    }
    expect(importModule).toHaveBeenCalledOnce()

    importModule.mockImplementationOnce(async () => ({ default: 'fresh' }))
    await expect(loadWebViewScriptModule(scriptPath, options)).resolves.toEqual(
      { default: 'fresh' },
    )
    expect(importModule).toHaveBeenCalledTimes(2)
    expect(importModule.mock.calls[1]?.[0]).toBe(`${scriptPath}?bldr_retry=1`)
  })
})

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

  it('reports a missing root plugin asset after module import fails', async () => {
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(404, 'missing plugin asset body', {
        'content-type': 'text/plain',
        'X-Bldr-Fetch-Source': 'plugin-assets',
        'X-Bldr-Runtime-Fetch-Error': 'plugin-asset-missing',
        'X-Bldr-Plugin-Asset-Fetch-Result': 'missing',
      }),
    )
    const importModule = vi.fn(async () => {
      throw new TypeError('Failed to fetch dynamically imported module')
    })

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
      expect.objectContaining({
        cache: 'no-store',
        signal: expect.any(AbortSignal),
      }),
    )
    expect(importModule).toHaveBeenCalledOnce()
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
    const importedModule = { default: 'component' }
    const moduleLoad = deferred<typeof importedModule>()
    const importStarted = deferred<void>()
    const fetchRootAsset = vi.fn<typeof fetch>()
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

    expect(fetchRootAsset).not.toHaveBeenCalled()
    expect(importModule).toHaveBeenCalledOnce()

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

      expect(fetchRootAsset).not.toHaveBeenCalled()
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

      expect(fetchRootAsset).toHaveBeenCalledOnce()
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

  it('imports a root plugin module without a diagnostic probe', async () => {
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

    expect(fetchRootAsset).not.toHaveBeenCalled()
    expect(globalThis.__bldrWebViewRootAssetStatus).toMatchObject({
      status: 0,
      ok: true,
      classification: 'imported',
    })
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

  it('rejects bypassed root plugin asset responses after module import fails', async () => {
    const fetchRootAsset = vi.fn(async () =>
      pluginAssetResponse(200, '<!doctype html><title>loading</title>', {
        'content-type': 'text/html',
      }),
    )
    const importModule = vi.fn(async () => {
      throw new TypeError('Failed to fetch dynamically imported module')
    })

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
    expect(importModule).toHaveBeenCalledOnce()
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

describe('reloadForFailedModuleGraph', () => {
  const liveHeaders = {
    'content-type': 'text/javascript',
    'X-Bldr-Fetch-Source': 'plugin-assets',
    'X-Bldr-Plugin-Asset-Fetch-Result': 'live',
  }

  async function failWithLiveRoot(scriptPath: string): Promise<Error> {
    const failure = new TypeError(
      `Failed to fetch dynamically imported module: ${scriptPath}`,
    )
    await expect(
      loadWebViewScriptModule(scriptPath, {
        fetchRootAsset: vi.fn(async () =>
          pluginAssetResponse(200, 'export default 1', liveHeaders),
        ),
        importModule: vi.fn(async () => {
          throw failure
        }),
      }),
    ).rejects.toBe(failure)
    return failure
  }

  it('reloads once per root script until an import of it succeeds', async () => {
    const scriptPath = '/b/pa/spacewave-app/v/app/App-graph-reload.mjs'
    sessionStorage.clear()
    try {
      const reload = vi.fn()
      const first = await failWithLiveRoot(scriptPath)
      expect(reloadForFailedModuleGraph(first, reload)).toBe(true)
      expect(reload).toHaveBeenCalledOnce()

      // The reloaded document failing the same way must not loop.
      const second = await failWithLiveRoot(scriptPath)
      expect(reloadForFailedModuleGraph(second, reload)).toBe(false)
      expect(reload).toHaveBeenCalledOnce()

      await expect(
        loadWebViewScriptModule(scriptPath, {
          importModule: vi.fn(async () => ({ default: 'component' })),
        }),
      ).resolves.toEqual({ default: 'component' })

      const third = await failWithLiveRoot(scriptPath)
      expect(reloadForFailedModuleGraph(third, reload)).toBe(true)
      expect(reload).toHaveBeenCalledTimes(2)
    } finally {
      sessionStorage.clear()
      globalThis.__bldrWebViewModuleImportError = undefined
      globalThis.__bldrWebViewRootAssetStatus = undefined
    }
  })

  it('does not reload when the root asset probe was not live', async () => {
    const scriptPath = '/b/pa/spacewave-app/v/app/App-graph-missing.mjs'
    sessionStorage.clear()
    try {
      const failure = new TypeError(
        `Failed to fetch dynamically imported module: ${scriptPath}`,
      )
      await expect(
        loadWebViewScriptModule(scriptPath, {
          fetchRootAsset: vi.fn(async () =>
            pluginAssetResponse(404, 'missing', {
              'X-Bldr-Fetch-Source': 'plugin-assets',
              'X-Bldr-Plugin-Asset-Fetch-Result': 'missing',
            }),
          ),
          importModule: vi.fn(async () => {
            throw failure
          }),
        }),
      ).rejects.toBeInstanceOf(WebViewRootAssetLoadError)

      const reload = vi.fn()
      expect(reloadForFailedModuleGraph(failure, reload)).toBe(false)
      expect(reload).not.toHaveBeenCalled()
    } finally {
      sessionStorage.clear()
      globalThis.__bldrWebViewModuleImportError = undefined
      globalThis.__bldrWebViewRootAssetStatus = undefined
    }
  })
})

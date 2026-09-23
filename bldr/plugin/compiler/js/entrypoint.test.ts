import { describe, expect, test, vi } from 'vitest'
import { retryWithAbort } from '@aptre/bldr'

import {
  entrypointRetryOpts,
  isEntrypointLifecycleRetry,
  loadBackendEntrypoints,
  loadWebPkgs,
  startBackendEntrypoint,
} from './entrypoint.js'

describe('plugin JS entrypoint retry logging', () => {
  test('classifies lifecycle retry errors', () => {
    const err = new Error('stream reset')
    err.name = 'StreamResetError'

    expect(isEntrypointLifecycleRetry(err)).toBe(true)
    expect(isEntrypointLifecycleRetry(new Error('stream reset'))).toBe(true)
    expect(isEntrypointLifecycleRetry(new Error('ERR_RPC_ABORT'))).toBe(true)
    expect(isEntrypointLifecycleRetry(new Error('different'))).toBe(false)
  })

  test('does not use generic retry warning for stream resets', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    let attempts = 0
    const controller = new AbortController()

    try {
      const result = retryWithAbort(
        controller.signal,
        async () => {
          attempts++
          if (attempts === 1) {
            const err = new Error('stream reset')
            err.name = 'StreamResetError'
            throw err
          }
        },
        entrypointRetryOpts('error configuring web view handlers'),
      )

      await vi.advanceTimersByTimeAsync(500)
      await result

      expect(attempts).toBe(2)
      expect(warn).not.toHaveBeenCalled()
      expect(error).not.toHaveBeenCalled()
    } finally {
      controller.abort()
      warn.mockRestore()
      error.mockRestore()
      vi.useRealTimers()
    }
  })

  test('does not use generic retry warning for RPC aborts', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    let attempts = 0
    const controller = new AbortController()

    try {
      const result = retryWithAbort(
        controller.signal,
        async () => {
          attempts++
          if (attempts === 1) {
            throw new Error('ERR_RPC_ABORT')
          }
        },
        entrypointRetryOpts('error configuring web view handlers'),
      )

      await vi.advanceTimersByTimeAsync(500)
      await result

      expect(attempts).toBe(2)
      expect(warn).not.toHaveBeenCalled()
      expect(error).not.toHaveBeenCalled()
    } finally {
      controller.abort()
      warn.mockRestore()
      error.mockRestore()
      vi.useRealTimers()
    }
  })

  test('logs unexpected retry errors with entrypoint context', async () => {
    vi.useFakeTimers()
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    let attempts = 0
    const controller = new AbortController()

    try {
      const result = retryWithAbort(
        controller.signal,
        async () => {
          attempts++
          if (attempts === 1) {
            throw new Error('boom')
          }
        },
        entrypointRetryOpts('error configuring web view handlers'),
      )

      await vi.advanceTimersByTimeAsync(500)
      await result

      expect(error).toHaveBeenCalledWith(
        'error configuring web view handlers: boom',
      )
      expect(error).toHaveBeenCalledWith(expect.any(Error))
    } finally {
      controller.abort()
      error.mockRestore()
      vi.useRealTimers()
    }
  })
})

describe('plugin JS backend entrypoint startup', () => {
  test('rejects an asset import without an exact worker binding', async () => {
    const load = vi.fn()
    const api = {
      startInfo: { pluginId: 'colors' },
      utils: { pluginAssetHttpPath: vi.fn() },
    }
    await expect(
      startBackendEntrypoint(
        { importPath: '/assets/backend.js' },
        api as never,
        new AbortController().signal,
        load,
      ),
    ).rejects.toThrow('immutable manifest binding')
    expect(load).not.toHaveBeenCalled()
    expect(api.utils.pluginAssetHttpPath).not.toHaveBeenCalled()
  })

  test('does not wait for long-lived backend lifecycle promises before startup resolves', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const backendAPI = {
      startInfo: { pluginId: 'spacewave-app', manifestRoot: 'old-manifest' },
      utils: {
        pluginAssetHttpPath: (pluginId: string, path: string) =>
          '/b/pa/' + pluginId + '/' + path,
      },
    }
    const abortController = new AbortController()
    const entrypointFn = vi.fn(() => new Promise<void>(() => {}))

    try {
      await expect(
        startBackendEntrypoint(
          { importPath: '/assets/backend.js', importName: 'default' },
          backendAPI as never,
          abortController.signal,
          async () => ({ default: entrypointFn }),
        ),
      ).resolves.toBeUndefined()

      expect(entrypointFn).toHaveBeenCalledOnce()
      expect(debug).toHaveBeenCalledWith(
        'Executing backend entrypoint: /b/pa/spacewave-app/manifest/old-manifest/backend.js#default',
      )
    } finally {
      abortController.abort()
      debug.mockRestore()
    }
  })

  test('waits for declared backend startup lifecycle before startup resolves', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const backendAPI = {
      startInfo: { pluginId: 'spacewave-notes', manifestRoot: 'old-manifest' },
      utils: {
        pluginAssetHttpPath: (pluginId: string, path: string) =>
          '/b/pa/' + pluginId + '/' + path,
      },
    }
    const abortController = new AbortController()
    let resolveStartup!: () => void
    const startup = new Promise<void>((resolve) => {
      resolveStartup = resolve
    })
    const done = new Promise<void>(() => {})
    let resolved = false

    try {
      const result = startBackendEntrypoint(
        { importPath: '/assets/backend.js', importName: 'default' },
        backendAPI as never,
        abortController.signal,
        async () => ({
          default: () => ({ startup, done }),
        }),
      )
      result.then(() => {
        resolved = true
      })

      await Promise.resolve()
      expect(resolved).toBe(false)

      resolveStartup()
      await result
      expect(resolved).toBe(true)
      expect(debug).toHaveBeenCalledWith(
        'Executing backend entrypoint: /b/pa/spacewave-notes/manifest/old-manifest/backend.js#default',
      )
    } finally {
      abortController.abort()
      debug.mockRestore()
    }
  })

  test('rejects declared backend startup lifecycle failures', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    const backendAPI = {
      startInfo: { pluginId: 'spacewave-notes', manifestRoot: 'old-manifest' },
      utils: {
        pluginAssetHttpPath: (pluginId: string, path: string) =>
          '/b/pa/' + pluginId + '/' + path,
      },
    }
    const abortController = new AbortController()

    try {
      await expect(
        startBackendEntrypoint(
          { importPath: '/assets/backend.js', importName: 'default' },
          backendAPI as never,
          abortController.signal,
          async () => ({
            default: () => ({
              startup: Promise.reject(new Error('startup failed')),
            }),
          }),
        ),
      ).rejects.toThrow('startup failed')
    } finally {
      abortController.abort()
      debug.mockRestore()
      error.mockRestore()
    }
  })

  test('observes backend lifecycle failures after startup', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    const backendAPI = {
      startInfo: { pluginId: 'spacewave-app', manifestRoot: 'old-manifest' },
      utils: {
        pluginAssetHttpPath: (pluginId: string, path: string) =>
          '/b/pa/' + pluginId + '/' + path,
      },
    }
    const abortController = new AbortController()

    try {
      await startBackendEntrypoint(
        { importPath: '/assets/backend.js', importName: 'default' },
        backendAPI as never,
        abortController.signal,
        async () => ({
          default: () => Promise.reject(new Error('late failure')),
        }),
      )
      await Promise.resolve()

      expect(error).toHaveBeenCalledWith(
        'Backend entrypoint failed after startup /b/pa/spacewave-app/manifest/old-manifest/backend.js#default: late failure',
      )
      expect(error).toHaveBeenCalledWith(expect.any(Error))
    } finally {
      abortController.abort()
      debug.mockRestore()
      error.mockRestore()
    }
  })

  test('loads only selected backend startup entrypoints', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const backendAPI = {
      startInfo: { pluginId: 'spacewave-app', manifestRoot: 'old-manifest' },
      utils: {
        pluginAssetHttpPath: (pluginId: string, path: string) =>
          '/b/pa/' + pluginId + '/' + path,
      },
    }
    const abortController = new AbortController()
    const imported: string[] = []

    try {
      await loadBackendEntrypoints(
        backendAPI as never,
        abortController.signal,
        [{ importPath: '/assets/notes.js', importName: 'default' }],
        async (importPath) => {
          imported.push(importPath)
          return { default: vi.fn() }
        },
      )

      expect(imported).toEqual([
        '/b/pa/spacewave-app/manifest/old-manifest/notes.js',
      ])
      expect(imported).not.toContain(
        '/b/pa/spacewave-app/manifest/old-manifest/vm.js',
      )
    } finally {
      abortController.abort()
      debug.mockRestore()
    }
  })

  test('propagates backend startup failures before readiness can be reported', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    const backendAPI = {
      startInfo: { pluginId: 'spacewave-app', manifestRoot: 'old-manifest' },
      utils: {
        pluginAssetHttpPath: (pluginId: string, path: string) =>
          '/b/pa/' + pluginId + '/' + path,
      },
    }
    const abortController = new AbortController()

    try {
      await expect(
        loadBackendEntrypoints(
          backendAPI as never,
          abortController.signal,
          [{ importPath: '/assets/notes.js', importName: 'default' }],
          async () => ({
            default: () => ({
              startup: Promise.reject(new Error('startup failed')),
            }),
          }),
        ),
      ).rejects.toThrow('startup failed')
    } finally {
      abortController.abort()
      debug.mockRestore()
      error.mockRestore()
    }
  })

  test('rejects missing backend entrypoint exports before readiness can be reported', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    const backendAPI = {
      startInfo: { pluginId: 'spacewave-app', manifestRoot: 'old-manifest' },
      utils: {
        pluginAssetHttpPath: (pluginId: string, path: string) =>
          '/b/pa/' + pluginId + '/' + path,
      },
    }
    const abortController = new AbortController()

    try {
      await expect(
        loadBackendEntrypoints(
          backendAPI as never,
          abortController.signal,
          [{ importPath: '/assets/notes.js', importName: 'start' }],
          async () => ({ default: vi.fn() }),
        ),
      ).rejects.toThrow(
        "Backend entrypoint function 'start' not found or not a function",
      )
    } finally {
      abortController.abort()
      debug.mockRestore()
      error.mockRestore()
    }
  })
})

describe('plugin JS frontend startup', () => {
  test('resolves web package readiness without closing the serving stream', async () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const abortController = new AbortController()
    const { promise: webPkgsRequested, resolve: markWebPkgsRequested } =
      Promise.withResolvers<void>()
    const { promise: webPkgsReady, resolve: markWebPkgsReady } =
      Promise.withResolvers<void>()
    const { promise: streamClosed, resolve: markStreamClosed } =
      Promise.withResolvers<void>()
    const events: string[] = []
    let closed = false

    const webPlugin = {
      HandleWebPkgsViaPluginAssets: vi.fn((request, signal?: AbortSignal) => {
        events.push(`request:${request.handlePluginId}`)
        markWebPkgsRequested()
        return (async function* () {
          await webPkgsReady
          events.push('ready')
          yield { body: { case: 'ready' as const, value: true } }
          const { promise: aborted, resolve } = Promise.withResolvers<void>()
          signal?.addEventListener('abort', () => resolve(), { once: true })
          await aborted
          closed = true
          markStreamClosed()
        })()
      }),
    }

    try {
      const ready = loadWebPkgs(
        'spacewave-app',
        webPlugin as never,
        abortController.signal,
        undefined,
        {
          webPkgsPath: 'web-pkgs',
          webPkgIdList: ['sonner'],
        },
      )
      let resolved = false
      ready.then(() => {
        resolved = true
      })

      await webPkgsRequested
      expect(resolved).toBe(false)

      markWebPkgsReady()
      await ready

      expect(resolved).toBe(true)
      expect(closed).toBe(false)
      expect(webPlugin.HandleWebPkgsViaPluginAssets).toHaveBeenCalledOnce()
      expect(events).toEqual(['request:spacewave-app', 'ready'])

      abortController.abort()
      await streamClosed
    } finally {
      abortController.abort()
      debug.mockRestore()
    }
  })
})

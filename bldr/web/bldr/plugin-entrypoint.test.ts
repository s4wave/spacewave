import { describe, expect, it, vi } from 'vitest'
import { HandleStreamCtr, type HandleStreamFunc } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'

import { startWorkerPluginEntrypoint } from './plugin-entrypoint.js'

describe('startWorkerPluginEntrypoint', () => {
  it('preserves legacy promise startup semantics', async () => {
    const lifetime = Promise.withResolvers<void>()
    let started = false
    const start = startWorkerPluginEntrypoint(
      () => lifetime.promise,
      { handleStreamCtr: new HandleStreamCtr() } as unknown as BackendAPI,
      new AbortController().signal,
      undefined,
      vi.fn(),
    ).then(() => {
      started = true
    })

    await Promise.resolve()
    expect(started).toBe(false)

    lifetime.resolve()
    await start
    expect(started).toBe(true)
  })

  it('waits for lifecycle startup and reports process failure', async () => {
    const startup = Promise.withResolvers<void>()
    const done = Promise.withResolvers<void>()
    const reportRuntimeFailure = vi.fn()
    let started = false
    const start = startWorkerPluginEntrypoint(
      () => ({ startup: startup.promise, done: done.promise }),
      { handleStreamCtr: new HandleStreamCtr() } as unknown as BackendAPI,
      new AbortController().signal,
      undefined,
      reportRuntimeFailure,
    ).then(() => {
      started = true
    })

    await Promise.resolve()
    expect(started).toBe(false)

    startup.resolve()
    await start
    expect(started).toBe(true)

    const err = new Error('plugin process failed')
    done.reject(err)
    await Promise.resolve()
    expect(reportRuntimeFailure).toHaveBeenCalledWith(err)
  })

  it('propagates lifecycle startup failure', async () => {
    const err = new Error('plugin startup failed')

    await expect(
      startWorkerPluginEntrypoint(
        () => ({ startup: Promise.reject(err) }),
        { handleStreamCtr: new HandleStreamCtr() } as unknown as BackendAPI,
        new AbortController().signal,
        undefined,
        vi.fn(),
      ),
    ).rejects.toBe(err)
  })

  it('answers streams as not found when the plugin serves none', async () => {
    const handleStreamCtr = new HandleStreamCtr()
    await startWorkerPluginEntrypoint(
      () => ({ startup: Promise.resolve() }),
      { handleStreamCtr } as unknown as BackendAPI,
      new AbortController().signal,
      undefined,
      vi.fn(),
    )
    expect(handleStreamCtr.value).toBeDefined()
  })

  it('keeps the handler the plugin registered', async () => {
    const handleStreamCtr = new HandleStreamCtr()
    const handler: HandleStreamFunc = vi.fn()
    await startWorkerPluginEntrypoint(
      (api) => {
        api.handleStreamCtr.set(handler)
        return { startup: Promise.resolve() }
      },
      { handleStreamCtr } as unknown as BackendAPI,
      new AbortController().signal,
      undefined,
      vi.fn(),
    )
    expect(handleStreamCtr.value).toBe(handler)
  })
})

import { describe, expect, it } from 'vitest'
import { startBackendEntrypoint } from '../../bldr/plugin/host/wazero-quickjs/lifecycle.js'
import type { BackendAPI } from '../../bldr/sdk/plugin.js'

const api = {} as BackendAPI

describe('startBackendEntrypoint', () => {
  it('waits for lifecycle startup and returns its completion', async () => {
    const startup = Promise.withResolvers<void>()
    const done = Promise.withResolvers<void>()
    const started = startBackendEntrypoint(
      () => ({ startup: startup.promise, done: done.promise }),
      api,
      new AbortController().signal,
    )
    let resolved = false
    void started.then(() => {
      resolved = true
    })
    await Promise.resolve()
    expect(resolved).toBe(false)
    startup.resolve()
    await expect(started).resolves.toEqual({ done: done.promise })
    done.resolve()
  })

  it('waits for legacy promise entrypoints', async () => {
    const entrypoint = Promise.withResolvers<void>()
    const started = startBackendEntrypoint(
      () => entrypoint.promise,
      api,
      new AbortController().signal,
    )
    let resolved = false
    void started.then(() => {
      resolved = true
    })
    await Promise.resolve()
    expect(resolved).toBe(false)
    entrypoint.resolve()
    await expect(started).resolves.toBeUndefined()
  })
})

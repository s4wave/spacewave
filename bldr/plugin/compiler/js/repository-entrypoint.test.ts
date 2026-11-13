import { expect, test, vi } from 'vitest'
import { HandleStreamCtr } from 'starpc'
import type { BackendAPI } from '@aptre/bldr-sdk'

import { PluginHostResourceServiceClient } from '../../../sdk/plugin/host/host_srpc.pb.js'
import { createPluginEntrypoint } from '../../../web/bldr/plugin-entrypoint.js'

test('repository entrypoint uses compiled configuration without build-time globals', async () => {
  // Complete capability registration through the same client as ordinary plugins.
  const complete = vi
    .spyOn(
      PluginHostResourceServiceClient.prototype,
      'CompleteInitialCapabilityRegistration',
    )
    .mockResolvedValue({})
  const release = vi.fn()
  const api = {
    pluginHost: {
      GetPluginInfo: vi
        .fn()
        .mockResolvedValue({ pluginId: 'repository-fixture' }),
    },
    resourceClient: {
      accessRootResource: vi.fn().mockResolvedValue({ client: {}, release }),
    },
    handleStreamCtr: new HandleStreamCtr(),
  } as unknown as BackendAPI
  const signal = new AbortController().signal

  // Empty module sets still complete startup without any compiler or injected constants.
  try {
    const entrypoint = createPluginEntrypoint({ webPluginId: 'web' })
    await entrypoint(api, signal).startup
    expect(complete).toHaveBeenCalledWith({}, signal)
    expect(release).toHaveBeenCalledOnce()
  } finally {
    complete.mockRestore()
  }
})

test('a QuickJS plugin answers calls before a browser shows its views', async () => {
  // Register capabilities at once; the web plugin never reports running, as on
  // a serve with no browser attached.
  const complete = vi
    .spyOn(
      PluginHostResourceServiceClient.prototype,
      'CompleteInitialCapabilityRegistration',
    )
    .mockResolvedValue({})
  const debug = vi.spyOn(console, 'debug').mockImplementation(() => {})
  const handleStreamCtr = new HandleStreamCtr()
  const api = {
    pluginHost: {
      GetPluginInfo: vi.fn().mockResolvedValue({ pluginId: 'viewer-fixture' }),
      LoadPlugin: () => ({
        [Symbol.asyncIterator]: () => ({ next: () => new Promise(() => {}) }),
      }),
    },
    resourceClient: {
      accessRootResource: vi
        .fn()
        .mockResolvedValue({ client: {}, release: vi.fn() }),
    },
    handleStreamCtr,
  } as unknown as BackendAPI
  const abort = new AbortController()
  Reflect.set(globalThis, 'std', {})

  // Startup waits on the views, but the backend serves the not-found mux, so
  // Activation.Check is answered and a swap replaces the worker in place.
  try {
    const entrypoint = createPluginEntrypoint({
      webPluginId: 'web',
      frontendEntrypoints: [{ setHtmlLinks: { setLinks: {} } }],
    })
    let started = false
    void Promise.resolve(entrypoint(api, abort.signal).startup).then(
      () => (started = true),
      () => {},
    )
    await vi.waitFor(() => expect(handleStreamCtr.value).toBeDefined())
    expect(started).toBe(false)
  } finally {
    abort.abort()
    Reflect.deleteProperty(globalThis, 'std')
    complete.mockRestore()
    debug.mockRestore()
  }
})

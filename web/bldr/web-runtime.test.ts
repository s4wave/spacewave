import { describe, expect, it, vi } from 'vitest'

import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebRuntime, WebRuntimeClientChannelStreamOpts } from './web-runtime.js'

describe('WebRuntime', () => {
  it('allows web runtime streams to stay idle', () => {
    expect(WebRuntimeClientChannelStreamOpts.keepAliveMs).toBeUndefined()
    expect(WebRuntimeClientChannelStreamOpts.idleTimeoutMs).toBeUndefined()
  })

  it('rejects pending waiters when a client is invalidated', async () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const waitForClient = runtime.waitForClient('electron-init')

    runtime.invalidateClient(
      'electron-init',
      new Error('renderer gone: crashed'),
    )

    await expect(waitForClient).rejects.toThrow('renderer gone: crashed')
  })

  it('removes active clients when invalidated', () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const { port1 } = new MessageChannel()

    runtime.handleClient(
      {
        clientUuid: 'electron-init',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      port1,
    )
    expect(runtime.lookupClient('electron-init')).not.toBeNull()

    runtime.invalidateClient(
      'electron-init',
      new Error('navigation started: app://index.html'),
    )

    expect(runtime.lookupClient('electron-init')).toBeNull()
  })

  it('closes descendant streams when invalidating a client generation', () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const { port1 } = new MessageChannel()

    runtime.handleClient(
      {
        clientUuid: 'electron-init-gen-1',
        logicalClientId: 'electron-init',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      port1,
    )

    const client = runtime.lookupClient('electron-init') as {
      childStreams: Set<{ close: (err?: Error) => void }>
    } | null
    expect(client).not.toBeNull()

    const close = vi.fn()
    client!.childStreams.add({ close })

    runtime.invalidateClient(
      'electron-init',
      new Error('navigation started: app://index.html'),
    )

    expect(close).toHaveBeenCalledTimes(1)
    expect(close.mock.calls[0]?.[0]).toBeInstanceOf(Error)
  })

  it('routes generated runtime clients through a stable logical id', async () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const { port1 } = new MessageChannel()

    runtime.handleClient(
      {
        clientUuid: 'electron-init-gen-2',
        logicalClientId: 'electron-init',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      port1,
    )

    await expect(runtime.waitForClient('electron-init')).resolves.toBe(
      runtime.lookupClient('electron-init'),
    )
    expect(runtime.lookupClient('electron-init-gen-2')).toBeNull()
  })

  it('lets the latest document generation re-register after refresh invalidation', async () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const { port1 } = new MessageChannel()
    runtime.handleClient(
      {
        clientUuid: 'electron-init-gen-1',
        logicalClientId: 'electron-init',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      port1,
    )
    const first = runtime.lookupClient('electron-init')

    runtime.invalidateClient(
      'electron-init',
      new Error('navigation started: app://index.html'),
    )
    expect(runtime.lookupClient('electron-init')).toBeNull()

    const waiter = runtime.waitForClient('electron-init')
    const { port1: port2 } = new MessageChannel()
    runtime.handleClient(
      {
        clientUuid: 'electron-init-gen-2',
        logicalClientId: 'electron-init',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      port2,
    )

    const second = runtime.lookupClient('electron-init')
    await expect(waiter).resolves.toBe(second)
    expect(second).not.toBeNull()
    expect(second).not.toBe(first)
  })
})

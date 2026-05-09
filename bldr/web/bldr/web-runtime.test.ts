import { describe, expect, it, vi } from 'vitest'
import {
  ChannelStream,
  Client as SRPCClient,
  StreamConn,
  combineUint8ArrayListTransform,
  type ChannelPort,
} from 'starpc'
import { pipe } from 'it-pipe'

import { Client as ResourceClient } from '../../sdk/resource/client.js'
import { ResourceServiceClient } from '../../sdk/resource/resource_srpc.pb.js'
import { DesktopTrayResourceServiceClient } from '../../desktop/tray/tray_srpc.pb.js'
import { DesktopTrayIconState } from '../../desktop/tray/tray.pb.js'
import { DesktopRuntimeResourceServiceClient } from '../electron/desktop-runtime/desktop-runtime_srpc.pb.js'
import {
  DesktopRuntimeHealth,
  DesktopRuntimeLifecycle,
} from '../electron/desktop-runtime/desktop-runtime.pb.js'
import { DesktopRuntimeResource } from '../electron/main/desktop-runtime.js'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebRuntimeClient as WebRuntimeServiceClient } from '../runtime/runtime_srpc.pb.js'
import {
  isClosedStreamWriteError,
  logWebRuntimeMessage,
  WebRuntime,
  WebRuntimeClientChannelStreamOpts,
} from './web-runtime.js'

class MemoryChannel {
  private handler: ((ev: { data: unknown }) => void) | null = null
  private readonly pending: unknown[] = []

  public set onmessage(handler: ((ev: { data: unknown }) => void) | null) {
    this.handler = handler
    this.flush()
  }

  public get onmessage(): ((ev: { data: unknown }) => void) | null {
    return this.handler
  }

  public constructor(private peer?: MemoryChannel) {}

  public setPeer(peer: MemoryChannel): void {
    this.peer = peer
  }

  public postMessage(data: unknown): void {
    this.peer?.deliver(data)
  }

  public close(): void {
    this.handler = null
    this.pending.length = 0
  }

  private deliver(data: unknown): void {
    if (!this.handler) {
      this.pending.push(data)
      return
    }
    queueMicrotask(() => this.handler?.({ data }))
  }

  private flush(): void {
    while (this.handler && this.pending.length > 0) {
      const data = this.pending.shift()
      queueMicrotask(() => this.handler?.({ data }))
    }
  }
}

function createChannelPortPair(): [ChannelPort, ChannelPort] {
  const clientRx = new MemoryChannel()
  const serverRx = new MemoryChannel()
  const clientTx = new MemoryChannel(serverRx)
  const serverTx = new MemoryChannel(clientRx)
  clientRx.setPeer(serverTx)
  serverRx.setPeer(clientTx)
  return [
    { tx: clientTx, rx: clientRx } as unknown as ChannelPort,
    { tx: serverTx, rx: serverRx } as unknown as ChannelPort,
  ]
}

function connectRuntimeServer(runtime: WebRuntime): {
  client: SRPCClient
  close(): Promise<void>
} {
  const serverTasks: Promise<void>[] = []
  const clientConn = new StreamConn()
  const serverConn = new StreamConn(runtime.getWebRuntimeServer(), {
    direction: 'inbound',
  })
  const [clientPort, serverPort] = createChannelPortPair()
  const clientStream = new ChannelStream('client', clientPort)
  const serverStream = new ChannelStream('server', serverPort)
  serverTasks.push(
    pipe(
      clientStream,
      clientConn,
      combineUint8ArrayListTransform(),
      clientStream,
    )
      .catch((err: unknown) => clientConn.close(toError(err)))
      .then(() => clientConn.close()),
    pipe(
      serverStream,
      serverConn,
      combineUint8ArrayListTransform(),
      serverStream,
    )
      .catch((err: unknown) => serverConn.close(toError(err)))
      .then(() => serverConn.close()),
  )
  return {
    client: new SRPCClient(clientConn.buildOpenStreamFunc()),
    async close() {
      clientConn.close()
      serverConn.close()
      await Promise.allSettled(serverTasks)
    },
  }
}

describe('WebRuntime', () => {
  it('allows web runtime streams to stay idle', () => {
    expect(WebRuntimeClientChannelStreamOpts.keepAliveMs).toBeUndefined()
    expect(WebRuntimeClientChannelStreamOpts.idleTimeoutMs).toBeUndefined()
  })

  it('treats closed log streams as teardown', () => {
    const err = new Error('write EPIPE') as Error & { code: string }
    err.code = 'EPIPE'
    const log = vi.spyOn(console, 'log').mockImplementation(() => {
      throw err
    })
    try {
      expect(isClosedStreamWriteError(err)).toBe(true)
      expect(() => logWebRuntimeMessage('closing')).not.toThrow()
    } finally {
      log.mockRestore()
    }
  })

  it('keeps non-stream log failures fatal', () => {
    const err = new Error('unexpected console failure') as Error & {
      code: string
    }
    err.code = 'OTHER'
    const log = vi.spyOn(console, 'log').mockImplementation(() => {
      throw err
    })
    try {
      expect(isClosedStreamWriteError(err)).toBe(false)
      expect(() => logWebRuntimeMessage('closing')).toThrow(err)
    } finally {
      log.mockRestore()
    }
  })

  it('exposes registered services through the process-lifetime runtime server', async () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const desktopResource = new DesktopRuntimeResource({
      openOrFocusMainWindow: vi.fn(),
      quitDesktopRuntime: vi.fn(),
    })
    runtime.registerServerExtension(desktopResource.resourceServer)

    const controller = new AbortController()
    const runtimeConn = connectRuntimeServer(runtime)
    const srpcClient = runtimeConn.client
    const resourceClient = new ResourceClient(
      new ResourceServiceClient(srpcClient),
      controller.signal,
    )

    const rootRef = await resourceClient.accessRootResource()
    const service = new DesktopRuntimeResourceServiceClient(rootRef.client)
    const tray = new DesktopTrayResourceServiceClient(rootRef.client)
    const iter = service.WatchDesktopState({})[Symbol.asyncIterator]()
    const trayIter = tray.WatchDesktopTray({})[Symbol.asyncIterator]()

    await expect(iter.next()).resolves.toMatchObject({
      value: {
        state: {
          statusText: 'Running',
          health: DesktopRuntimeHealth.HEALTHY,
          lifecycle: DesktopRuntimeLifecycle.RUNNING,
        },
      },
      done: false,
    })
    await expect(trayIter.next()).resolves.toMatchObject({
      value: {
        state: {
          iconState: DesktopTrayIconState.NORMAL,
        },
      },
      done: false,
    })

    await service.SetDesktopState({
      state: {
        statusText: 'Projected',
        health: DesktopRuntimeHealth.ACTIVE,
        lifecycle: DesktopRuntimeLifecycle.RUNNING,
      },
    })
    await expect(iter.next()).resolves.toMatchObject({
      value: {
        state: {
          statusText: 'Projected',
          health: DesktopRuntimeHealth.ACTIVE,
          lifecycle: DesktopRuntimeLifecycle.RUNNING,
        },
      },
      done: false,
    })
    expect(desktopResource.getState()).toMatchObject({
      statusText: 'Projected',
      health: DesktopRuntimeHealth.ACTIVE,
    })

    await iter.return?.()
    await trayIter.return?.()
    rootRef.release()
    resourceClient.dispose()
    controller.abort()
    await runtimeConn.close()
  })

  it('flushes the browser index cache through the runtime RPC', async () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const runtimeConn = connectRuntimeServer(runtime)
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response('updated index', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    try {
      const service = new WebRuntimeServiceClient(runtimeConn.client)

      await service.FlushIndexCache({})

      expect(fetchMock).toHaveBeenCalledTimes(1)
      expect(fetchMock.mock.calls[0][0]).toBe(
        new URL('/b/__index.html', globalThis.location.href).toString(),
      )
      expect(fetchMock.mock.calls[0][1]).toEqual({ cache: 'reload' })
    } finally {
      await runtimeConn.close()
      vi.unstubAllGlobals()
    }
  })

  it('rejects a failed browser index cache flush', async () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const runtimeConn = connectRuntimeServer(runtime)
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('unavailable', { status: 503 })),
    )

    try {
      const service = new WebRuntimeServiceClient(runtimeConn.client)

      await expect(service.FlushIndexCache({})).rejects.toThrow(
        'browser index cache refresh failed: status=503',
      )
    } finally {
      await runtimeConn.close()
      vi.unstubAllGlobals()
    }
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

function toError(err: unknown): Error {
  if (err instanceof Error) {
    return err
  }
  return new Error(String(err))
}

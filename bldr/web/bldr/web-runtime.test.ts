import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ChannelStream,
  Client as SRPCClient,
  StreamConn,
  combineUint8ArrayListTransform,
  openRpcStream,
  type ChannelPort,
} from 'starpc'
import { pipe } from 'it-pipe'

import { Client as ResourceClient } from '../../sdk/resource/client.js'
import { ResourceServiceClient } from '../../sdk/resource/resource_srpc.pb.js'
import { DesktopTrayResourceServiceClient } from '../../desktop/tray/tray_srpc.pb.js'
import { DesktopTrayIconState } from '../../desktop/tray/tray.pb.js'
import {
  DesktopCLIInstallResourceServiceClient,
  DesktopRuntimeResourceServiceClient,
} from '../electron/desktop-runtime/desktop-runtime_srpc.pb.js'
import {
  DesktopCLIInstallStatus,
  DesktopRuntimeHealth,
  DesktopRuntimeLifecycle,
} from '../electron/desktop-runtime/desktop-runtime.pb.js'
import { DesktopRuntimeResource } from '../electron/main/desktop-runtime.js'
import { WebRuntimeClientType } from '../runtime/runtime.pb.js'
import { WebRuntimeClient as WebRuntimeServiceClient } from '../runtime/runtime_srpc.pb.js'
import { buildWebDocumentLockName } from '../runtime/runtime.js'
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

class TransferMessagePort {
  private handler:
    | ((ev: { data: unknown; ports: MessagePort[] }) => void)
    | null = null
  private readonly pending: { data: unknown; ports: MessagePort[] }[] = []
  private peer?: TransferMessagePort

  public set onmessage(
    handler: ((ev: { data: unknown; ports: MessagePort[] }) => void) | null,
  ) {
    this.handler = handler
    this.flush()
  }

  public get onmessage():
    | ((ev: { data: unknown; ports: MessagePort[] }) => void)
    | null {
    return this.handler
  }

  public setPeer(peer: TransferMessagePort): void {
    this.peer = peer
  }

  public postMessage(data: unknown, ports: MessagePort[] = []): void {
    this.peer?.deliver({ data, ports })
  }

  public start(): void {
    this.flush()
  }

  public close(): void {
    this.handler = null
    this.pending.length = 0
  }

  private deliver(event: { data: unknown; ports: MessagePort[] }): void {
    if (!this.handler) {
      this.pending.push(event)
      return
    }
    queueMicrotask(() => this.handler?.(event))
  }

  private flush(): void {
    while (this.handler && this.pending.length > 0) {
      const event = this.pending.shift()
      queueMicrotask(() => event && this.handler?.(event))
    }
  }
}

class TransferMessageChannel {
  public readonly port1: MessagePort
  public readonly port2: MessagePort

  constructor() {
    const left = new TransferMessagePort()
    const right = new TransferMessagePort()
    left.setPeer(right)
    right.setPeer(left)
    this.port1 = left as unknown as MessagePort
    this.port2 = right as unknown as MessagePort
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

function installFakeMessageChannel(): {
  channels: Array<{ port1: MessagePort; port2: MessagePort }>
  port(): MessagePort
} {
  const channels: Array<{ port1: MessagePort; port2: MessagePort }> = []
  class FakeMessagePort {
    public onmessage: ((ev: MessageEvent) => void) | null = null
    public postMessage = vi.fn()
    public start = vi.fn()
    public close = vi.fn()
  }
  class FakeMessageChannel {
    public readonly port1 = new FakeMessagePort() as unknown as MessagePort
    public readonly port2 = new FakeMessagePort() as unknown as MessagePort

    public constructor() {
      channels.push({ port1: this.port1, port2: this.port2 })
    }
  }
  vi.stubGlobal('MessagePort', FakeMessagePort)
  vi.stubGlobal('MessageChannel', FakeMessageChannel)
  return {
    channels,
    port() {
      return new FakeMessagePort() as unknown as MessagePort
    },
  }
}

function installControllableWebLocks(): {
  release(name: string): void
  requestCount(name: string): number
} {
  const releasers = new Map<string, () => void>()
  const counts = new Map<string, number>()
  vi.stubGlobal('navigator', {
    locks: {
      request: (
        name: string,
        opts: { signal?: AbortSignal },
        cb: () => unknown,
      ) => {
        counts.set(name, (counts.get(name) ?? 0) + 1)
        return new Promise<unknown>((resolve, reject) => {
          const abort = () => reject(new DOMException('aborted', 'AbortError'))
          opts.signal?.addEventListener('abort', abort, { once: true })
          releasers.set(name, () => {
            opts.signal?.removeEventListener('abort', abort)
            resolve(cb())
          })
        })
      },
    },
  })
  return {
    release(name: string): void {
      releasers.get(name)?.()
    },
    requestCount(name: string): number {
      return counts.get(name) ?? 0
    },
  }
}

async function flushMessages(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
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
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

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
    const cliInstall = new DesktopCLIInstallResourceServiceClient(
      rootRef.client,
    )
    const iter = service.WatchDesktopState({})[Symbol.asyncIterator]()
    const trayIter = tray.WatchDesktopTray({})[Symbol.asyncIterator]()
    const cliInstallIter = cliInstall
      .WatchCLIInstallState({})
      [Symbol.asyncIterator]()

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
    await expect(cliInstallIter.next()).resolves.toMatchObject({
      value: {
        state: {
          status: DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UNKNOWN,
          generation: 1n,
          actions: [
            { id: 'recheck', generation: 1n },
            { id: 'open-settings', generation: 1n },
          ],
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
    await cliInstallIter.return?.()
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

  it('forwards startup accounting and plugin roots only to documents', () => {
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const documentChannel = new MessageChannel()
    const workerChannel = new MessageChannel()
    runtime.handleClient(
      {
        clientUuid: 'document-1',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      documentChannel.port1,
    )
    runtime.handleClient(
      {
        clientUuid: 'worker-1',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      },
      workerChannel.port1,
    )
    const documentClient = runtime.lookupClient('document-1')
    const workerClient = runtime.lookupClient('worker-1')
    expect(documentClient).not.toBeNull()
    expect(workerClient).not.toBeNull()
    const documentPost = vi.spyOn(documentClient!, 'postMessage')
    const workerPost = vi.spyOn(workerClient!, 'postMessage')

    runtime.broadcastStartupMark('manifest-copy.done', {
      blocksSeen: 3,
      logicalSourceBytes: 1024,
    })

    expect(documentPost).toHaveBeenCalledWith({
      startupMark: {
        label: 'manifest-copy.done',
        detail: { blocksSeen: 3, logicalSourceBytes: 1024 },
      },
    })
    expect(workerPost).not.toHaveBeenCalled()
    documentPost.mockClear()
    runtime.broadcastPluginManifestRoot('spacewave-app', '2abc')
    expect(documentPost).toHaveBeenCalledWith({
      pluginManifestRoot: {
        pluginId: 'spacewave-app',
        rootHash: '2abc',
      },
    })
    expect(workerPost).not.toHaveBeenCalled()
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

  it('keeps runtime-to-client stream opens pending until client invalidation', async () => {
    vi.useFakeTimers()
    const fake = installFakeMessageChannel()
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)

    runtime.handleClient(
      {
        clientUuid: 'document-1',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      fake.port(),
    )

    const client = runtime.lookupClient('document-1') as {
      openStream(): Promise<unknown>
    } | null
    expect(client).not.toBeNull()

    let settled = false
    const openPromise = client!.openStream()
    openPromise.then(
      () => {
        settled = true
      },
      () => {
        settled = true
      },
    )

    await vi.advanceTimersByTimeAsync(30_000)

    expect(settled).toBe(false)

    runtime.invalidateClient('document-1', new Error('document lock released'))

    await expect(openPromise).rejects.toThrow('closed')
  })

  it('tracks opened runtime-to-client streams under the client generation', async () => {
    const fake = installFakeMessageChannel()
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)

    runtime.handleClient(
      {
        clientUuid: 'document-1',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      fake.port(),
    )

    const client = runtime.lookupClient('document-1') as {
      childStreams: Set<{ close: (err?: Error) => void }>
      openStream(): Promise<unknown>
    } | null
    expect(client).not.toBeNull()

    const openPromise = client!.openStream()
    await Promise.resolve()
    expect(fake.channels).toHaveLength(1)
    fake.channels[0].port1.onmessage?.({
      data: { from: 'document', ack: true, opened: true },
    } as MessageEvent)

    await expect(openPromise).resolves.toBeDefined()
    expect(client!.childStreams.size).toBe(1)

    runtime.invalidateClient('document-1', new Error('document lock released'))

    expect(client!.childStreams.size).toBe(0)
    expect(fake.channels[0].port1.close).toHaveBeenCalled()
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

  it('routes WebWorkerRpc streams through a registered worker client', async () => {
    vi.stubGlobal('MessagePort', TransferMessagePort)
    vi.stubGlobal('MessageChannel', TransferMessageChannel)

    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const runtimeConn = connectRuntimeServer(runtime)
    const service = new WebRuntimeServiceClient(runtimeConn.client)
    const { port1, port2 } = new MessageChannel()
    const workerTasks: Promise<void>[] = []

    try {
      port2.onmessage = (ev) => {
        const data = ev.data
        if (typeof data !== 'object' || !data.openStream) {
          return
        }
        const remotePort = ev.ports[0]
        expect(remotePort).toBeDefined()
        const workerStream = new ChannelStream('worker', remotePort, {
          ...WebRuntimeClientChannelStreamOpts,
          remoteOpen: true,
        })
        const requestPromise = (async () => {
          // Channel streams are duplex; waiting for EOF or breaking a for-await
          // loop can close the response path before the worker writes back.
          const result =
            await workerStream.source[Symbol.asyncIterator]().next()
          return result.value ?? new Uint8Array()
        })()
        const responsePromise = workerStream.sink(
          (async function* () {
            const request = await requestPromise
            yield new Uint8Array([42, ...request])
          })(),
        )
        workerTasks.push(
          Promise.all([requestPromise, responsePromise]).then(() => {}),
        )
      }
      port2.start()

      runtime.handleClient(
        {
          clientUuid: 'plugin/spacewave-notes',
          clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
        },
        port1,
      )

      const stream = await openRpcStream(
        'plugin/spacewave-notes',
        service.WebWorkerRpc.bind(service),
        true,
      )
      await stream.sink(
        (async function* () {
          yield new Uint8Array([7, 8, 9])
        })(),
      )

      const response = stream.source[Symbol.asyncIterator]()
      await expect(response.next()).resolves.toEqual({
        done: false,
        value: new Uint8Array([42, 7, 8, 9]),
      })
      await expect(response.next()).resolves.toEqual({
        done: true,
        value: undefined,
      })

      await Promise.all(workerTasks)
      port2.close()
    } finally {
      await runtimeConn.close()
      vi.unstubAllGlobals()
    }
  })

  it('marks document clients suspect on Web Lock grant without deleted status', async () => {
    const locks = installControllableWebLocks()
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const statusSpy = vi.spyOn(runtime.statusStream, 'pushChangeEvent')
    const { port1 } = new MessageChannel()

    runtime.handleClient(
      {
        clientUuid: 'document-1',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      port1,
    )
    const client = runtime.lookupClient('document-1')
    expect(client).not.toBeNull()
    expect(Reflect.get(client!, 'state')).toBe('active')
    const close = vi.fn()
    Reflect.get(client!, 'childStreams').add({ close })
    statusSpy.mockClear()

    Reflect.apply(Reflect.get(client!, 'armWebLock'), client!, [])
    const lockName = buildWebDocumentLockName('document-1')
    locks.release(lockName)
    await flushMessages()

    expect(runtime.lookupClient('document-1')).toBe(client)
    expect(Reflect.get(client!, 'state')).toBe('suspect')
    expect(close).not.toHaveBeenCalled()
    expect(statusSpy).not.toHaveBeenCalled()
    expect(locks.requestCount(lockName)).toBe(2)
  })

  it('same logical id usurp swaps channels without deleting the document', async () => {
    const locks = installControllableWebLocks()
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const statusSpy = vi.spyOn(runtime.statusStream, 'pushChangeEvent')
    const { port1: firstPort } = new MessageChannel()
    runtime.handleClient(
      {
        clientUuid: 'document-gen-1',
        logicalClientId: 'document',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      firstPort,
    )
    const first = runtime.lookupClient('document')
    expect(first).not.toBeNull()
    const close = vi.fn()
    Reflect.get(first!, 'childStreams').add({ close })
    Reflect.apply(Reflect.get(first!, 'armWebLock'), first!, [])
    locks.release(buildWebDocumentLockName('document-gen-1'))
    await flushMessages()
    expect(Reflect.get(first!, 'state')).toBe('suspect')

    const secondChannel = new MessageChannel()
    runtime.handleClient(
      {
        clientUuid: 'document-gen-2',
        logicalClientId: 'document',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      secondChannel.port1,
    )

    const second = runtime.lookupClient('document')
    expect(second).not.toBeNull()
    expect(second).not.toBe(first)
    expect(Reflect.get(first!, 'state')).toBe('closed')
    expect(Reflect.get(second!, 'state')).toBe('active')
    expect(close).toHaveBeenCalledTimes(1)
    const statuses = statusSpy.mock.calls.flatMap(
      ([status]) => status.webDocuments ?? [],
    )
    expect(statuses.filter((status) => status.deleted)).toHaveLength(0)
    expect(statuses.filter((status) => status.deleted === false)).toHaveLength(
      1,
    )
  })

  it('explicit close from suspect emits one deleted event', async () => {
    const locks = installControllableWebLocks()
    const runtime = new WebRuntime('runtime-1', vi.fn(), null, null)
    const statusSpy = vi.spyOn(runtime.statusStream, 'pushChangeEvent')
    const { port1 } = new MessageChannel()
    runtime.handleClient(
      {
        clientUuid: 'document-1',
        clientType: WebRuntimeClientType.WebRuntimeClientType_WEB_DOCUMENT,
      },
      port1,
    )
    const client = runtime.lookupClient('document-1')
    expect(client).not.toBeNull()
    Reflect.apply(Reflect.get(client!, 'armWebLock'), client!, [])
    locks.release(buildWebDocumentLockName('document-1'))
    await flushMessages()
    expect(Reflect.get(client!, 'state')).toBe('suspect')
    await Reflect.apply(Reflect.get(client!, 'onClientMessage'), client!, [
      { data: { close: true }, ports: [] },
    ])
    await flushMessages()

    expect(runtime.lookupClient('document-1')).toBeNull()
    expect(Reflect.get(client!, 'state')).toBe('closed')
    const deletedStatuses = statusSpy.mock.calls
      .flatMap(([status]) => status.webDocuments ?? [])
      .filter((status) => status.deleted)
    expect(deletedStatuses).toHaveLength(1)
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

import { pipe } from 'it-pipe'
import { pushable } from 'it-pushable'
import { MessagePortDuplex, PacketStream, castToError } from 'starpc'
import {
  callTinyGoExport,
  GoWasmProcess,
  retainTinyGoPluginStreamStoredBytes,
  syncTinyGoPluginStreamPendingDeliveries,
  tinyGoExport,
  tinyGoMemory,
  type TinyGoRuntime,
} from '../../runtime/wasm/go-process.js'
import { BackendAPI } from '@aptre/bldr-sdk'
import { PluginStartInfo } from '../../../plugin/plugin.pb.js'

interface Global {
  BLDR_BASE_URL: string
  BLDR_PLUGIN_START_INFO?: string
  BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?: (err: unknown) => void
  BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME?: (
    onMessage: (message: Uint8Array) => void,
    onClose: (errMsg?: string) => void,
    onResolve: (sink: GoPushableSink) => void,
    onReject: (errMsg: string) => void,
  ) => void
  BLDR_PLUGIN_SET_ACCEPT_STREAM?: (
    acceptStream: (localPort: MessagePort) => void,
  ) => void
}

type GoPushableSink = {
  push: (message: Uint8Array) => void
  end: () => void
}

// globalScope is globalThis but with the bldr globals.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const globalScope: Global = globalThis as any

// baseURL is the base URL to use for paths relative to this module.
const baseURL = import.meta?.url
globalScope.BLDR_BASE_URL = baseURL

const goCallbackQueue: (() => void)[] = []
let goCallbackScheduled = false
let goCallbackChannel: MessageChannel | undefined

// BLDR_PLUGIN_ENTRYPOINT is declared at build time by the plugin compiler.
declare const BLDR_PLUGIN_ENTRYPOINT: string
const pluginEntrypointPath = BLDR_PLUGIN_ENTRYPOINT

class WasmPluginGeneration {
  private readonly activeAcceptedStreams = new Set<
    MessagePortDuplex<Uint8Array>
  >()
  private terminalError?: Error
  private tinyGoStreamBridge?: TinyGoPluginStreamBridge

  public constructor(
    private readonly api: BackendAPI,
    private readonly abortSignal?: AbortSignal,
    private readonly runtimeWasmEnv?: Record<string, string>,
  ) {}

  public start(startInfo: PluginStartInfo) {
    const pluginStartInfoJsonB64 = btoa(PluginStartInfo.toJsonString(startInfo))
    globalScope.BLDR_PLUGIN_START_INFO = pluginStartInfoJsonB64
    const tinyGoStreamBridge = new TinyGoPluginStreamBridge(this.api, (err) =>
      this.fail(err),
    )
    this.tinyGoStreamBridge = tinyGoStreamBridge
    const goProcess = new GoWasmProcess(
      new URL(pluginEntrypointPath, baseURL).toString(),
      {
        argv: ['plugin.wasm'],
        env: {
          ...this.runtimeWasmEnv,
          BLDR_PLUGIN_START_INFO: pluginStartInfoJsonB64,
        },
        abortSignal: this.abortSignal,
        retry: false,
        tinyGoRuntimeImports: (go) => tinyGoStreamBridge.install(go),
      },
    )

    const result = goProcess.start()
    void result.then(
      () => {
        if (!this.abortSignal?.aborted) {
          this.fail(new Error('Go WASM process exited'))
        }
      },
      (err) => {
        if (!this.abortSignal?.aborted) {
          this.fail(err)
        }
      },
    )
  }

  public setAcceptStream(acceptStrm?: (localPort: MessagePort) => void) {
    if (this.terminalError) {
      this.installTerminalAcceptHandler(this.terminalError)
      return
    }
    if (!acceptStrm) {
      this.api.handleStreamCtr.set(undefined)
      return
    }

    this.api.handleStreamCtr.set(async (channel: PacketStream) => {
      if (this.terminalError) {
        throw this.terminalError
      }

      const messageChannel = new MessageChannel()
      let accepted = false
      try {
        acceptStrm(messageChannel.port1)
        accepted = true
      } finally {
        if (!accepted) {
          messageChannel.port1.close()
          messageChannel.port2.close()
        }
      }

      const duplex = new MessagePortDuplex<Uint8Array>(messageChannel.port2)
      this.activeAcceptedStreams.add(duplex)
      try {
        await pipe(channel, duplex, channel)
      } finally {
        this.activeAcceptedStreams.delete(duplex)
        closeMessagePortDuplex(duplex)
      }
    })
  }

  private fail(err: unknown) {
    if (this.terminalError) {
      return
    }

    const terminalError = castToError(err, 'Go WASM process failed')
    this.terminalError = terminalError
    console.warn('plugin-wasm: Go WASM process exited', terminalError)
    this.closeActiveAcceptedStreams()
    this.tinyGoStreamBridge?.closeAll()
    this.installTerminalAcceptHandler(terminalError)
    globalScope.BLDR_PLUGIN_REPORT_RUNTIME_FAILURE?.(terminalError)
  }

  private closeActiveAcceptedStreams() {
    for (const duplex of this.activeAcceptedStreams) {
      closeMessagePortDuplex(duplex)
    }
    this.activeAcceptedStreams.clear()
  }

  private installTerminalAcceptHandler(err: Error) {
    this.api.handleStreamCtr.set(async () => {
      throw err
    })
  }
}

class TinyGoPluginStreamBridge {
  private go?: TinyGoRuntime
  private nextStreamID = 1
  private nextBytesID = 1
  private readonly streams = new Map<number, TinyGoPluginStream>()
  private readonly storedBytes = new Map<number, Uint8Array>()
  private readonly storedByteReleases = new Map<number, () => void>()
  private readonly pendingDeliveries = new Map<number, TinyGoPluginDelivery>()
  private readonly encoder = new TextEncoder()

  public constructor(
    private readonly api: BackendAPI,
    private readonly reportFailure: (err: unknown) => void,
  ) {}

  public install(go: TinyGoRuntime): void {
    this.go = go
    const gojs = go.importObject['gojs']
    if (!gojs) {
      return
    }

    gojs['bldr.plugin.openStream'] ??= (opID: number) => {
      this.openStream(opID)
    }
    gojs['bldr.plugin.streamWrite'] ??= (
      streamID: number,
      ptr: number,
      len: number,
    ) => this.writeStream(streamID, ptr, len)
    gojs['bldr.plugin.streamClose'] ??= (streamID: number) =>
      this.closeStream(streamID)
    gojs['bldr.plugin.streamRelease'] ??= (streamID: number) => {
      this.releaseStream(streamID)
    }
    gojs['bldr.plugin.streamTakeBytes'] ??= (
      bytesID: number,
      ptr: number,
      len: number,
    ) => this.takeBytes(bytesID, ptr, len)
    gojs['bldr.plugin.streamDropBytes'] ??= (bytesID: number) =>
      this.dropBytes(bytesID) ? 1 : 0
    gojs['bldr.plugin.streamMessageHandled'] ??= (
      bytesID: number,
      delivered: number,
    ) => {
      this.resolveDelivery(bytesID, delivered !== 0)
    }
    gojs['bldr.plugin.setAcceptStreams'] ??= (enabled: number) => {
      this.setAcceptStreams(enabled !== 0)
    }
  }

  public closeAll(): void {
    for (const streamID of Array.from(this.streams.keys())) {
      this.releaseStream(streamID)
    }
    this.clearStoredBytes()
    this.resolveAllDeliveries(false)
    this.api.handleStreamCtr.set(undefined)
  }

  private openStream(opID: number): void {
    void (async () => {
      const packetStream = await this.api.openStream()
      const stream = this.createStream(packetStream)
      try {
        this.callExport('BLDR_PLUGIN_STREAM_OPEN_RESOLVE', opID, stream.id)
      } catch (err) {
        this.releaseStream(stream.id)
        throw err
      }
    })().catch((err) => {
      this.rejectOpenStream(opID, err)
    })
  }

  private rejectOpenStream(opID: number, err: unknown): void {
    try {
      const { id, len } = this.storeError(err)
      this.callExport('BLDR_PLUGIN_STREAM_OPEN_REJECT', opID, id, len)
    } catch (callbackErr) {
      this.reportFailure(callbackErr)
    }
  }

  private setAcceptStreams(enabled: boolean): void {
    if (!enabled) {
      this.api.handleStreamCtr.set(undefined)
      return
    }

    this.api.handleStreamCtr.set(async (packetStream: PacketStream) => {
      const stream = this.createStream(packetStream)
      try {
        this.callExport('BLDR_PLUGIN_STREAM_ACCEPT', stream.id)
      } catch (err) {
        this.releaseStream(stream.id)
        throw err
      }
    })
  }

  private createStream(packetStream: PacketStream): TinyGoPluginStream {
    const id = this.nextStreamID++
    const outbound = pushable<Uint8Array>({ objectMode: true })
    const stream: TinyGoPluginStream = {
      id,
      outbound,
      released: false,
    }
    this.streams.set(id, stream)

    void packetStream.sink(outbound).catch((err) => {
      this.closeFromJS(id, err)
    })
    void (async () => {
      try {
        for await (const message of packetStream.source) {
          if (!(await this.deliverMessage(id, message))) {
            return
          }
        }
        this.closeFromJS(id)
      } catch (err) {
        this.closeFromJS(id, err)
      }
    })()

    return stream
  }

  private async deliverMessage(
    streamID: number,
    message: Uint8Array,
  ): Promise<boolean> {
    if (!this.streams.has(streamID)) {
      return false
    }
    try {
      return await this.deliverStoredMessage(streamID, message)
    } catch (err) {
      this.releaseStream(streamID)
      this.reportFailure(err)
      return false
    }
  }

  private async deliverStoredMessage(
    streamID: number,
    message: Uint8Array,
  ): Promise<boolean> {
    const bytes = copyUint8Array(message)
    const bytesID = this.storeBytes(bytes)
    const delivered = this.awaitDelivery(streamID, bytesID)
    try {
      this.callExport(
        'BLDR_PLUGIN_STREAM_MESSAGE',
        streamID,
        bytesID,
        bytes.byteLength,
      )
    } catch (err) {
      this.resolveDelivery(bytesID, false)
      throw err
    }
    return await delivered
  }

  private closeFromJS(streamID: number, err?: unknown): void {
    if (!this.streams.has(streamID)) {
      return
    }
    try {
      if (err == null) {
        this.callExport('BLDR_PLUGIN_STREAM_CLOSE', streamID, 0, 0)
        return
      }
      const { id, len } = this.storeError(err)
      this.callExport('BLDR_PLUGIN_STREAM_CLOSE', streamID, id, len)
    } catch (callbackErr) {
      this.releaseStream(streamID)
      this.reportFailure(callbackErr)
    }
  }

  private writeStream(streamID: number, ptr: number, len: number): number {
    const stream = this.streams.get(streamID)
    if (!stream || stream.released) {
      return 0
    }
    try {
      const bytes = new Uint8Array(len)
      if (len !== 0) {
        bytes.set(
          new Uint8Array(tinyGoMemory(this.mustGo()).buffer, ptr >>> 0, len),
        )
      }
      stream.outbound.push(bytes)
      return 1
    } catch {
      return 0
    }
  }

  private closeStream(streamID: number): number {
    const stream = this.streams.get(streamID)
    if (!stream || stream.released) {
      return 0
    }
    try {
      stream.outbound.end()
      return 1
    } catch {
      return 0
    }
  }

  private releaseStream(streamID: number): void {
    const stream = this.streams.get(streamID)
    if (!stream || stream.released) {
      return
    }
    stream.released = true
    this.streams.delete(streamID)
    this.resolveStreamDeliveries(streamID, false)
    try {
      stream.outbound.end()
    } catch {
      // ignored: the sink may already be closed by the peer.
    }
  }

  private takeBytes(bytesID: number, ptr: number, len: number): number {
    const bytes = this.takeStoredBytes(bytesID)
    if (!bytes) {
      return 0
    }
    if (bytes.byteLength !== len) {
      return 0
    }
    if (len === 0) {
      return 1
    }
    try {
      new Uint8Array(tinyGoMemory(this.mustGo()).buffer, ptr >>> 0, len).set(
        bytes,
      )
      return 1
    } catch {
      return 0
    }
  }

  private dropBytes(bytesID: number): boolean {
    return this.deleteStoredBytes(bytesID)
  }

  private storeError(err: unknown): { id: number; len: number } {
    const bytes = this.encoder.encode(castToError(err).toString())
    return { id: this.storeBytes(bytes), len: bytes.byteLength }
  }

  private storeBytes(bytes: Uint8Array): number {
    const id = this.nextBytesID++
    this.storedBytes.set(id, bytes)
    this.storedByteReleases.set(
      id,
      retainTinyGoPluginStreamStoredBytes(this.mustGo(), bytes.byteLength),
    )
    return id
  }

  private awaitDelivery(streamID: number, bytesID: number): Promise<boolean> {
    return new Promise((resolve) => {
      this.pendingDeliveries.set(bytesID, { streamID, resolve })
      this.syncPendingDeliveries()
    })
  }

  private resolveDelivery(bytesID: number, delivered: boolean): void {
    this.deleteStoredBytes(bytesID)
    const delivery = this.pendingDeliveries.get(bytesID)
    if (!delivery) {
      return
    }
    this.pendingDeliveries.delete(bytesID)
    this.syncPendingDeliveries()
    delivery.resolve(delivered)
  }

  private resolveStreamDeliveries(streamID: number, delivered: boolean): void {
    for (const [bytesID, delivery] of this.pendingDeliveries) {
      if (delivery.streamID === streamID) {
        this.deleteStoredBytes(bytesID)
        this.pendingDeliveries.delete(bytesID)
        this.syncPendingDeliveries()
        delivery.resolve(delivered)
      }
    }
  }

  private resolveAllDeliveries(delivered: boolean): void {
    for (const [bytesID, delivery] of this.pendingDeliveries) {
      this.deleteStoredBytes(bytesID)
      this.pendingDeliveries.delete(bytesID)
      this.syncPendingDeliveries()
      delivery.resolve(delivered)
    }
  }

  private takeStoredBytes(bytesID: number): Uint8Array | undefined {
    const bytes = this.storedBytes.get(bytesID)
    this.deleteStoredBytes(bytesID)
    return bytes
  }

  private deleteStoredBytes(bytesID: number): boolean {
    const deleted = this.storedBytes.delete(bytesID)
    const release = this.storedByteReleases.get(bytesID)
    if (release) {
      this.storedByteReleases.delete(bytesID)
      release()
    }
    return deleted
  }

  private clearStoredBytes(): void {
    for (const release of this.storedByteReleases.values()) {
      release()
    }
    this.storedByteReleases.clear()
    this.storedBytes.clear()
  }

  private syncPendingDeliveries(): void {
    syncTinyGoPluginStreamPendingDeliveries(
      this.mustGo(),
      this.pendingDeliveries.size,
    )
  }

  private callExport(name: string, ...args: number[]): void {
    const go = this.mustGo()
    const fn = tinyGoExport(go, name)
    if (!fn) {
      throw new Error(`TinyGo stream export ${name} is not initialized`)
    }
    callTinyGoExport(go, fn, ...args)
  }

  private mustGo(): TinyGoRuntime {
    if (!this.go) {
      throw new Error('TinyGo stream bridge is not initialized')
    }
    return this.go
  }
}

type TinyGoPluginStream = {
  id: number
  outbound: ReturnType<typeof pushable<Uint8Array>>
  released: boolean
}

type TinyGoPluginDelivery = {
  streamID: number
  resolve: (delivered: boolean) => void
}

// Main function exported by this module.
export default async function main(
  api: BackendAPI,
  abortSignal?: AbortSignal,
  runtimeWasmEnv?: Record<string, string>,
): Promise<void> {
  const generation = new WasmPluginGeneration(api, abortSignal, runtimeWasmEnv)

  // The Go runtime will call this function to open outgoing streams.
  globalScope.BLDR_PLUGIN_OPEN_STREAM_TO_WEB_RUNTIME = (
    onMessage,
    onClose,
    onResolve,
    onReject,
  ): void => {
    void (async () => {
      const packetStream = await api.openStream()
      const packetSource = packetStream.source
      const push = pushable<Uint8Array>({ objectMode: true })
      let callbacksClosed = false
      const closeCallbacks = (errMsg?: string) => {
        if (callbacksClosed) {
          return
        }
        callbacksClosed = true
        deferGoCallback(() => {
          callGoCallback(() => {
            onClose(errMsg)
          })
        })
      }
      const deliverMessage = async (msg: Uint8Array): Promise<boolean> => {
        if (callbacksClosed) {
          return false
        }
        const released = await deferGoCallbackResult(() =>
          callGoCallback(() => {
            onMessage(msg)
          }),
        )
        if (released) {
          callbacksClosed = true
          return false
        }
        return true
      }
      queueMicrotask(async () => {
        try {
          for await (const msg of packetSource) {
            if (!(await deliverMessage(msg))) {
              push.end()
              return
            }
          }
          closeCallbacks()
        } catch (err) {
          const e = castToError(err)
          closeCallbacks(e.toString())
        }
      })

      queueMicrotask(() => {
        void packetStream.sink(push).catch((err) => {
          const e = castToError(err)
          closeCallbacks(e.toString())
        })
      })
      return {
        push: (message: Uint8Array) => {
          push.push(message)
        },
        end: () => {
          push.end()
        },
      }
    })().then(
      (sink) => {
        deferGoCallback(() => {
          const released = callGoCallback(() => {
            onResolve(sink)
          })
          if (released) {
            sink.end()
          }
        })
      },
      (err) => {
        deferGoCallback(() => {
          callGoCallback(() => {
            onReject(castToError(err).toString())
          })
        })
      },
    )
  }

  // The Go runtime will call this function to set a callback for incoming streams.
  globalScope.BLDR_PLUGIN_SET_ACCEPT_STREAM = (
    acceptStrm?: (localPort: MessagePort) => void,
  ) => {
    generation.setAcceptStream(acceptStrm)
  }

  // Start the Go plugin, passing the startInfo from the API
  generation.start(api.startInfo)
}

function closeMessagePortDuplex(duplex: MessagePortDuplex<Uint8Array>) {
  try {
    duplex.close()
  } catch {
    // ignored: the port may already be closed by the pipe.
  }
}

function copyUint8Array(bytes: Uint8Array): Uint8Array<ArrayBuffer> {
  const copy = new Uint8Array(bytes.byteLength)
  copy.set(bytes)
  return copy
}

function flushGoCallbacks(): void {
  goCallbackScheduled = false
  const callback = goCallbackQueue.shift()
  if (callback) {
    // TinyGo's asyncified runtime owns a single pending JS callback event.
    // Run one callback per task so stream delivery cannot re-enter the Go
    // runtime before the prior callback's resumed goroutines settle.
    callback()
  }
  if (goCallbackQueue.length !== 0) {
    scheduleGoCallbackFlush()
  }
}

function scheduleGoCallbackFlush(): void {
  if (goCallbackScheduled) {
    return
  }
  goCallbackScheduled = true
  if (typeof MessageChannel === 'function') {
    goCallbackChannel ??= new MessageChannel()
    goCallbackChannel.port1.onmessage = flushGoCallbacks
    goCallbackChannel.port2.postMessage(undefined)
    return
  }
  setTimeout(flushGoCallbacks, 0)
}

function deferGoCallback(callback: () => void): void {
  goCallbackQueue.push(callback)
  scheduleGoCallbackFlush()
}

function deferGoCallbackResult<T>(callback: () => T): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    deferGoCallback(() => {
      try {
        resolve(callback())
      } catch (err) {
        reject(err)
      }
    })
  })
}

function isReleasedGoCallbackError(err: unknown): boolean {
  if (typeof err === 'string') {
    return err === 'call to released function'
  }
  return err instanceof Error && err.message === 'call to released function'
}

function callGoCallback(cb: () => void): boolean {
  const consoleError = console.error
  let released = false
  console.error = (...args: unknown[]) => {
    // Go may release a callback before this owner invokes it. Filter that
    // known callback edge only inside the invocation, never for the whole worker.
    if (args.some(isReleasedGoCallbackError)) {
      released = true
      return
    }
    consoleError(...args)
  }
  try {
    cb()
  } catch (err) {
    if (isReleasedGoCallbackError(err)) {
      released = true
      return released
    }
    throw err
  } finally {
    console.error = consoleError
  }
  return released
}

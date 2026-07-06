import {
  Client,
  PacketStream,
  ChannelStream,
  castToError,
  HandleStreamFunc,
} from 'starpc'

import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import { ClientToWebRuntime, WebRuntimeToClient } from '../runtime/runtime.js'
import { WebRuntimeClientChannelStreamOpts } from './web-runtime.js'
import { markStartupBoundary } from './startup-marks.js'

// RuntimeClientGenerationState is the lifecycle state for one runtime client channel.
export type RuntimeClientGenerationState =
  | 'idle'
  | 'opening'
  | 'connected'
  | 'closed'
  | 'failed'

// RuntimeClientGenerationCloseReason records why a generation stopped.
export type RuntimeClientGenerationCloseReason =
  | 'not-started'
  | 'normal-close'
  | 'connect-failed'
  | 'runtime-disconnected'
  | 'relay-rerouted'

// RuntimeClientGenerationSnapshot is the public runtime-client generation view.
export interface RuntimeClientGenerationSnapshot {
  id: number
  state: RuntimeClientGenerationState
  webRuntimeId: string
  clientId: string
  logicalClientId?: string
  clientType: WebRuntimeClientType
  openedAtMs?: number
  connectedAtMs?: number
  closedAtMs?: number
  closeReason?: RuntimeClientGenerationCloseReason
  errorMessage?: string
  activeStreams: number
}

interface RuntimeClientGeneration extends Omit<
  RuntimeClientGenerationSnapshot,
  'activeStreams'
> {}

// RuntimeClientStreamOpenGateState is the lifecycle gate state before opening a stream.
export type RuntimeClientStreamOpenGateState =
  | 'ready'
  | 'unavailable'
  | 'closed'

// RuntimeClientStreamOpenGateResult describes whether stream-open may proceed.
export interface RuntimeClientStreamOpenGateResult {
  state: RuntimeClientStreamOpenGateState
  documentId?: string
  reason?: string
}

// RuntimeClientGenerationGateError is thrown when the stream-open gate rejects a generation.
export class RuntimeClientGenerationGateError extends Error {
  constructor(
    public readonly gate: RuntimeClientStreamOpenGateResult,
    clientId: string,
  ) {
    super(
      `WebRuntimeClient: ${clientId}: resume-ready ${gate.state}${gate.reason ? ': ' + gate.reason : ''}`,
    )
    this.name = 'RuntimeClientGenerationGateError'
  }
}

// RuntimeClientClosedError is the close error applied to runtime client
// generations and streams when the logical client tears down.
export class RuntimeClientClosedError extends Error {
  constructor(
    public readonly reason: RuntimeClientGenerationCloseReason,
    clientId: string,
    generationId: number,
  ) {
    super(
      `WebRuntimeClient: ${clientId}: runtime client generation ${generationId} closed: ${reason}`,
    )
    this.name = 'RuntimeClientClosedError'
  }
}

// isNormalRuntimeClientClose reports whether the error is a normal-close
// runtime client generation teardown rather than an unexpected failure.
// It also matches the message because the error can cross postMessage
// boundaries, which strip the class prototype.
export function isNormalRuntimeClientClose(err: unknown): boolean {
  if (err instanceof RuntimeClientClosedError) {
    return err.reason === 'normal-close'
  }
  return (
    err instanceof Error &&
    /runtime client generation \d+ closed: normal-close$/.test(err.message)
  )
}

function isRelayReroutedRuntimeClientClose(err: unknown): boolean {
  if (err instanceof RuntimeClientClosedError) {
    return err.reason === 'relay-rerouted'
  }
  return (
    err instanceof Error &&
    /runtime client generation \d+ closed: relay-rerouted$/.test(err.message)
  )
}

// OpenChannelFn opens the MessagePort to the WebRuntime.
export type OpenChannelFn = (init: WebRuntimeClientInit) => Promise<MessagePort>

// HandleDisconnectedFn handles when the web runtime client was disconnected.
export type HandleDisconnectedFn = (err?: Error) => Promise<void>

// WaitForStreamOpenGateFn waits for a startup gate that must be ready before
// stream-open can proceed.
export type WaitForStreamOpenGateFn =
  () => Promise<RuntimeClientStreamOpenGateResult | void>

export interface RerouteChannelOptions {
  reconnect?: boolean
}

class RuntimeClientPacketStream implements PacketStream {
  public readonly source: PacketStream['source']
  public readonly sink: PacketStream['sink']
  private sourceClosed = false
  private sinkClosed = false
  private released = false

  constructor(
    private readonly inner: ChannelStream,
    private readonly release: () => void,
  ) {
    this.source = this.wrapSource(inner.source)
    this.sink = async (source) => {
      try {
        await inner.sink(source)
      } finally {
        this.sinkClosed = true
        this.releaseIfBothDirectionsClosed()
      }
    }
  }

  public close(error?: Error): void {
    this.releaseOnce()
    this.inner.close(error)
  }

  private async *wrapSource(
    source: PacketStream['source'],
  ): PacketStream['source'] {
    try {
      yield* source
    } finally {
      this.sourceClosed = true
      this.releaseIfBothDirectionsClosed()
    }
  }

  private releaseIfBothDirectionsClosed(): void {
    // A clean duplex half-close is not a dead stream. Keep it generation-owned
    // until both halves finish or close() tears down the still-live side.
    if (this.sourceClosed && this.sinkClosed) {
      this.releaseOnce()
    }
  }

  private releaseOnce(): void {
    if (this.released) {
      return
    }
    this.released = true
    this.release()
  }
}

// WebRuntimeClient opens streams via a remote WebRuntime.
export class WebRuntimeClient {
  // rpcClient is the rpc client to the web runtime via openStream.
  public readonly rpcClient: Client
  // clientChannel is the active message port to the remote.
  private clientChannel?: MessagePort
  // reconnectingClientChannel is the in-flight reconnect shared by callers.
  private reconnectingClientChannel?: Promise<MessagePort>
  private nextGenerationId = 1
  private generation: RuntimeClientGeneration
  private generationAbortController?: AbortController
  private readonly activeStreams = new Set<RuntimeClientPacketStream>()

  constructor(
    public readonly webRuntimeId: string,
    public readonly clientId: string,
    public readonly clientType: WebRuntimeClientType,
    private openClientCh: OpenChannelFn,
    private handleIncomingStream: HandleStreamFunc | null,
    private handleDisconnected: HandleDisconnectedFn | null,
    private disableWebLocks?: boolean,
    private logicalClientId?: string,
    private waitForStreamOpenGateFn?: WaitForStreamOpenGateFn,
  ) {
    this.rpcClient = new Client(this.openStream.bind(this))
    this.generation = {
      id: 0,
      state: 'idle',
      webRuntimeId,
      clientId,
      logicalClientId,
      clientType,
      closeReason: 'not-started',
    }
  }

  // getRuntimeGenerationSnapshot returns the current runtime-client generation.
  public getRuntimeGenerationSnapshot(): RuntimeClientGenerationSnapshot {
    return {
      ...this.generation,
      activeStreams: this.activeStreams.size,
    }
  }

  // waitConn opens and waits for the connection to be ready.
  public async waitConn() {
    await this.getClientChannelWithRetry()
  }

  // openStream opens a RPC stream with the WebRuntimeHost.
  // the remote service depends on the WebRuntimeClientType.
  // relay-rerouted is a stale browser route, not a logical client close; retry
  // the stream open on the next generation instead of surfacing it to callers.
  public async openStream(): Promise<PacketStream> {
    for (;;) {
      try {
        return await this.openStreamOnCurrentRoute()
      } catch (err) {
        if (!isRelayReroutedRuntimeClientClose(err)) {
          throw err
        }
      }
    }
  }

  private async openStreamOnCurrentRoute(): Promise<PacketStream> {
    const clientPort = await this.getClientChannelWithRetry()
    const generationId = this.generation.id

    await this.waitForStreamOpenGate(generationId)

    const streamChannel = new MessageChannel()
    const streamConn = new ChannelStream(
      this.clientId,
      streamChannel.port1,
      WebRuntimeClientChannelStreamOpts,
    )
    const stream = this.trackRuntimeStream(streamConn, generationId)
    try {
      const msg: ClientToWebRuntime = {
        openStream: true,
      }
      clientPort.postMessage(msg, [streamChannel.port2])
      // Do not add a timer timeout here. Browser background tabs throttle
      // timers aggressively, so liveness is owned by generation close instead.
      await this.waitForRuntimeGeneration(
        generationId,
        streamConn.waitRemoteOpen,
      )
      if (!streamConn.isOpen) {
        throw new Error(
          `WebRuntimeClient: ${this.clientId}: stream closed before remote open`,
        )
      }
      return stream
    } catch (errAny) {
      const err = castToError(
        errAny,
        `WebRuntimeClient: ${this.clientId}: opening stream with host failed`,
      )
      stream.close(err)
      throw err
    }
  }

  // close closes the client channel and signals the close to the remote.
  // note: the client can still be used again after calling close().
  public close() {
    const reconnectingClientChannel = this.reconnectingClientChannel
    this.reconnectingClientChannel = undefined
    reconnectingClientChannel?.catch(() => {})
    void this.closeClientChannel('normal-close')
  }

  // rerouteChannel drops the stale relay path after its relaying WebDocument
  // closed. Unlike close(), it does not tell the runtime the client is going
  // away: the logical client stays alive on the runtime. Stream opens that are
  // waiting on the stale generation retry on the next route, and established
  // streams keep their transferred ports unless the browser closes them.
  public async rerouteChannel(opts: RerouteChannelOptions = {}): Promise<void> {
    const reconnectingClientChannel = this.reconnectingClientChannel
    this.reconnectingClientChannel = undefined
    reconnectingClientChannel?.catch(() => {})
    markStartupBoundary('runtime.client-channel-reroute-start', {
      source: 'browser',
      runtimeId: this.webRuntimeId,
      clientId: this.clientId,
      clientType: this.clientType,
      reconnect: opts.reconnect !== false,
    })
    await this.closeClientChannel('relay-rerouted')
    markStartupBoundary('runtime.client-channel-rerouted', {
      source: 'browser',
      runtimeId: this.webRuntimeId,
      clientId: this.clientId,
      clientType: this.clientType,
      reconnect: opts.reconnect !== false,
    })
    if (opts.reconnect === false) {
      return
    }
    markStartupBoundary('runtime.client-channel-reconnect-start', {
      source: 'browser',
      runtimeId: this.webRuntimeId,
      clientId: this.clientId,
      clientType: this.clientType,
    })
    // Re-establish through a surviving WebDocument so the next plugin-asset
    // fetch finds a ready relay. Best-effort: a failed reconnect is retried
    // lazily by the next openStream.
    void this.waitConn().catch(() => {})
  }

  // openClientChannel opens the client MessagePort to the WebRuntimeHost.
  // waits for a connected ack from the runtime before caching the port.
  private async openClientChannel(): Promise<MessagePort> {
    if (this.clientChannel) {
      return this.clientChannel
    }

    const generation = this.beginRuntimeGeneration()
    let port: MessagePort
    try {
      port = await this.openClientCh({
        webRuntimeId: this.webRuntimeId,
        clientUuid: this.clientId,
        logicalClientId: this.logicalClientId,
        clientType: this.clientType,
        disableWebLocks: this.disableWebLocks,
      })
    } catch (errAny) {
      const err = castToError(
        errAny,
        `WebRuntimeClient: ${this.clientId}: failed to open runtime client channel`,
      )
      this.finishRuntimeGeneration(
        generation.id,
        'failed',
        'connect-failed',
        err,
      )
      throw err
    }
    markStartupBoundary('runtime.client-channel-opened', {
      source: 'browser',
      runtimeId: this.webRuntimeId,
      clientId: this.clientId,
      clientType: this.clientType,
    })

    try {
      await this.waitForRuntimeConnectedAck(port, generation.id)
    } catch (errAny) {
      port.close()
      const err = castToError(
        errAny,
        `WebRuntimeClient: ${this.clientId}: failed while waiting for runtime connected ack`,
      )
      if (
        this.generation.id === generation.id &&
        this.generation.state === 'opening'
      ) {
        this.finishRuntimeGeneration(
          generation.id,
          'failed',
          'connect-failed',
          err,
        )
      }
      throw err
    }
    this.connectRuntimeGeneration(generation.id)
    markStartupBoundary('runtime.client-channel-acked', {
      source: 'browser',
      runtimeId: this.webRuntimeId,
      clientId: this.clientId,
      clientType: this.clientType,
    })

    // Ack received. Switch to normal message handler and cache the port.
    port.onmessage = (ev) => {
      const data = ev.data
      if (typeof data !== 'object' || data === null) {
        return
      }
      this.handleMessage(data, ev.ports)
    }
    this.clientChannel = port

    // Tell the WebRuntime to start watching our Web Lock for disconnect detection.
    // This is sent after we've acquired the lock (in WebDocument constructor),
    // ensuring no race condition where WebRuntime acquires the lock first.
    if (!this.disableWebLocks) {
      const armMsg: ClientToWebRuntime = { armWebLock: true }
      port.postMessage(armMsg)
    }

    return port
  }

  // getClientChannelWithRetry shares a single reconnect sequence across all
  // callers so parallel RPCs converge on one recovered runtime channel.
  private async getClientChannelWithRetry(): Promise<MessagePort> {
    if (this.clientChannel) {
      return this.clientChannel
    }
    if (this.reconnectingClientChannel) {
      return this.reconnectingClientChannel
    }

    const reconnectPromise = this.openClientChannelWithRetryImpl().finally(
      () => {
        if (this.reconnectingClientChannel === reconnectPromise) {
          this.reconnectingClientChannel = undefined
        }
      },
    )
    this.reconnectingClientChannel = reconnectPromise
    return reconnectPromise
  }

  // openClientChannelWithRetryImpl opens one generation; events on the owning
  // Web Locks/generation decide whether that attempt stays pending or closes.
  private async openClientChannelWithRetryImpl(): Promise<MessagePort> {
    return this.openClientChannel()
  }

  private async waitForStreamOpenGate(generationId: number): Promise<void> {
    if (!this.waitForStreamOpenGateFn) {
      return
    }
    const gateResult = await this.waitForRuntimeGeneration(
      generationId,
      this.waitForStreamOpenGateFn(),
    )
    if (gateResult && gateResult.state !== 'ready') {
      throw new RuntimeClientGenerationGateError(gateResult, this.clientId)
    }
  }

  // handleMessage handles an incoming message from the WebRuntime.
  private async handleMessage(
    msg: WebRuntimeToClient,
    ports?: readonly MessagePort[],
  ) {
    if (msg.openStream && ports && ports.length) {
      await this.handleWebRuntimeOpenStream(ports[0])
    }
  }

  // handleWebRuntimeOpenStream handles an incoming request to open a stream.
  private async handleWebRuntimeOpenStream(remoteMsgPort: MessagePort) {
    const channel = new ChannelStream(this.clientId, remoteMsgPort, {
      ...WebRuntimeClientChannelStreamOpts,
      remoteOpen: true,
    })
    const stream = this.trackRuntimeStream(channel, this.generation.id)
    let err: Error | undefined
    if (!this.handleIncomingStream) {
      err = new Error(
        `${this.clientType.toString()}: handle stream: not implemented`,
      )
    } else {
      try {
        await this.handleIncomingStream(stream)
      } catch (e) {
        err = castToError(
          e,
          `${this.clientType.toString()}: handle stream: unknown error`,
        )
      }
    }
    if (err) {
      console.error(err.message)
      stream.close(err)
      return
    }
  }

  private beginRuntimeGeneration(): RuntimeClientGeneration {
    this.generationAbortController?.abort(
      new Error(
        `WebRuntimeClient: ${this.clientId}: runtime client generation ${this.generation.id} superseded`,
      ),
    )
    this.generationAbortController = new AbortController()
    const generation: RuntimeClientGeneration = {
      id: this.nextGenerationId++,
      state: 'opening',
      webRuntimeId: this.webRuntimeId,
      clientId: this.clientId,
      logicalClientId: this.logicalClientId,
      clientType: this.clientType,
      openedAtMs: performance.now(),
    }
    this.generation = generation
    return generation
  }

  private connectRuntimeGeneration(generationId: number): void {
    if (this.generation.id !== generationId) {
      return
    }
    this.generation = {
      ...this.generation,
      state: 'connected',
      connectedAtMs: performance.now(),
      closeReason: undefined,
      errorMessage: undefined,
    }
  }

  private finishRuntimeGeneration(
    generationId: number,
    state: Extract<RuntimeClientGenerationState, 'closed' | 'failed'>,
    reason: RuntimeClientGenerationCloseReason,
    err?: Error,
  ): void {
    if (this.generation.id !== generationId) {
      return
    }
    const closeErr =
      err ?? new RuntimeClientClosedError(reason, this.clientId, generationId)
    this.generationAbortController?.abort(closeErr)
    this.generationAbortController = undefined
    this.generation = {
      ...this.generation,
      state,
      closedAtMs: performance.now(),
      closeReason: reason,
      errorMessage: err?.message,
    }
  }

  private async closeClientChannel(
    reason: RuntimeClientGenerationCloseReason,
    err?: Error,
  ): Promise<void> {
    const hadClientChannel = !!this.clientChannel
    const hadActiveStreams = this.activeStreams.size > 0
    const openingGeneration = this.generation.state === 'opening'
    if (
      !hadClientChannel &&
      !hadActiveStreams &&
      !openingGeneration &&
      reason === 'normal-close'
    ) {
      return
    }
    const generationId = this.generation.id
    if (reason !== 'relay-rerouted') {
      const closeErr =
        err ?? new RuntimeClientClosedError(reason, this.clientId, generationId)
      this.closeRuntimeStreams(closeErr)
    }
    if (this.clientChannel) {
      try {
        if (reason === 'normal-close') {
          const msg: ClientToWebRuntime = { close: true }
          this.clientChannel.postMessage(msg)
        }
      } finally {
        this.clientChannel.close()
        this.clientChannel = undefined
      }
    }
    const state = reason === 'normal-close' ? 'closed' : 'failed'
    this.finishRuntimeGeneration(generationId, state, reason, err)
    if (
      this.handleDisconnected &&
      reason !== 'relay-rerouted' &&
      (hadClientChannel || err)
    ) {
      await this.handleDisconnected(err)
    }
  }

  private async waitForRuntimeConnectedAck(
    port: MessagePort,
    generationId: number,
  ): Promise<void> {
    const ackPromise = new Promise<void>((resolve) => {
      port.onmessage = (ev) => {
        const data = ev.data
        if (typeof data === 'object' && data !== null && data.connected) {
          resolve()
        }
      }
      port.start()
    })
    try {
      // Do not timeout this ack with setTimeout. Background tabs can delay
      // browser timers long enough to falsely kill a valid retained runtime.
      await this.waitForRuntimeGeneration(generationId, ackPromise)
    } finally {
      port.onmessage = null
    }
  }

  private async waitForRuntimeGeneration<T>(
    generationId: number,
    promise: Promise<T>,
  ): Promise<T> {
    const closed = this.runtimeGenerationClosedPromise(generationId)
    try {
      return await Promise.race([promise, closed.promise])
    } finally {
      closed.cleanup()
    }
  }

  private runtimeGenerationClosedPromise(generationId: number): {
    promise: Promise<never>
    cleanup: () => void
  } {
    const signal = this.generationAbortController?.signal
    let cleanup = () => {}
    const promise = new Promise<never>((_, reject) => {
      const rejectClosed = () => {
        reject(
          signal?.reason instanceof Error
            ? signal.reason
            : this.runtimeGenerationClosedError(generationId),
        )
      }
      if (!signal || this.generation.id !== generationId || signal.aborted) {
        rejectClosed()
        return
      }
      signal.addEventListener('abort', rejectClosed, { once: true })
      cleanup = () => signal.removeEventListener('abort', rejectClosed)
    })
    return { promise, cleanup }
  }

  private runtimeGenerationClosedError(generationId: number): Error {
    return new Error(
      `WebRuntimeClient: ${this.clientId}: runtime client generation ${generationId} closed`,
    )
  }

  private trackRuntimeStream(
    channel: ChannelStream,
    generationId: number,
  ): RuntimeClientPacketStream {
    const stream = new RuntimeClientPacketStream(channel, () => {
      this.activeStreams.delete(stream)
    })
    if (generationId === this.generation.id) {
      this.activeStreams.add(stream)
    }
    return stream
  }

  private closeRuntimeStreams(err: Error): void {
    const streams = Array.from(this.activeStreams)
    this.activeStreams.clear()
    for (const stream of streams) {
      stream.close(err)
    }
  }
}

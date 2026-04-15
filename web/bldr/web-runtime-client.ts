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
import { timeoutPromise } from './timeout.js'
import { WebRuntimeClientChannelStreamOpts } from './web-runtime.js'

// OpenChannelFn opens the MessagePort to the WebRuntime.
export type OpenChannelFn = (init: WebRuntimeClientInit) => Promise<MessagePort>

// HandleDisconnectedFn handles when the web runtime client was disconnected.
export type HandleDisconnectedFn = (err?: Error) => Promise<void>

// WebRuntimeClient opens streams via a remote WebRuntime.
export class WebRuntimeClient {
  // rpcClient is the rpc client to the web runtime via openStream.
  public readonly rpcClient: Client
  // clientChannel is the active message port to the remote.
  private clientChannel?: MessagePort
  // reconnectingClientChannel is the in-flight reconnect shared by callers.
  private reconnectingClientChannel?: Promise<MessagePort>

  constructor(
    public readonly webRuntimeId: string,
    public readonly clientId: string,
    public readonly clientType: WebRuntimeClientType,
    private openClientCh: OpenChannelFn,
    private handleIncomingStream: HandleStreamFunc | null,
    private handleDisconnected: HandleDisconnectedFn | null,
    private disableWebLocks?: boolean,
  ) {
    this.rpcClient = new Client(this.openStream.bind(this))
  }

  // waitConn opens and waits for the connection to be ready.
  public async waitConn() {
    await this.getClientChannelWithRetry()
  }

  // openStream opens a RPC stream with the WebRuntimeHost.
  // the remote service depends on the WebRuntimeClientType.
  //
  // times out if the client does not ack within 3 seconds.
  public async openStream(): Promise<PacketStream> {
    // retry several times
    let err: Error | undefined
    for (let attempt = 0; attempt < 3; attempt++) {
      const clientPort = await this.getClientChannelWithRetry()
      const streamChannel = new MessageChannel()
      const streamConn = new ChannelStream(
        this.clientId,
        streamChannel.port1,
        WebRuntimeClientChannelStreamOpts,
      )
      const msg: ClientToWebRuntime = {
        openStream: true,
      }
      clientPort.postMessage(msg, [streamChannel.port2])
      await Promise.race([streamConn.waitRemoteOpen, timeoutPromise(1500)])
      if (!streamConn.isOpen) {
        streamConn.close()
        const msg = `WebRuntimeClient: ${this.clientId}: timeout opening stream with host`
        err = new Error(msg)
        console.warn(msg)
        if (this.clientChannel === clientPort) {
          this.clientChannel.close()
          this.clientChannel = undefined
          if (this.handleDisconnected) {
            await this.handleDisconnected(err)
          }
        }
        // try again shortly.
        await timeoutPromise(100)
        continue
      }

      // very verbose
      // console.log(`WebRuntimeClient: ${this.clientId}: opened stream with host`)
      return streamConn
    }

    err = new Error(
      `WebRuntimeClient: ${this.clientId}: unable to open stream with host${err ? ': ' + err : ''}`,
    )
    console.warn(err.message)
    throw err
  }

  // close closes the client channel and signals the close to the remote.
  // note: the client can still be used again after calling close().
  public close() {
    this.reconnectingClientChannel = undefined
    if (this.clientChannel) {
      const msg: ClientToWebRuntime = { close: true }
      this.clientChannel.postMessage(msg)
      this.clientChannel.close()
      this.clientChannel = undefined
      if (this.handleDisconnected) {
        this.handleDisconnected().catch(() => {})
      }
    }
  }

  // openClientChannel opens the client MessagePort to the WebRuntimeHost.
  // waits for a connected ack from the runtime before caching the port.
  private async openClientChannel(): Promise<MessagePort> {
    if (this.clientChannel) {
      return this.clientChannel
    }

    const port = await this.openClientCh({
      webRuntimeId: this.webRuntimeId,
      clientUuid: this.clientId,
      clientType: this.clientType,
      disableWebLocks: this.disableWebLocks,
    })

    // Wait for connected ack from the runtime before treating the port as live.
    // The ack is the first message sent by WebRuntimeClientInstance after
    // registration. Without this, reconnect can cache a dead MessagePort.
    const acked = await Promise.race([
      new Promise<true>((resolve) => {
        port.onmessage = (ev) => {
          const data = ev.data
          if (typeof data === 'object' && data.connected) {
            resolve(true)
          }
        }
        port.start()
      }),
      timeoutPromise(3000).then(() => false as const),
    ])

    if (!acked) {
      port.close()
      throw new Error(
        `WebRuntimeClient: ${this.clientId}: timeout waiting for runtime connected ack`,
      )
    }

    // Ack received. Switch to normal message handler and cache the port.
    port.onmessage = (ev) => {
      const data = ev.data
      if (typeof data !== 'object') {
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

    const reconnectPromise = this.openClientChannelWithRetryImpl().finally(() => {
      if (this.reconnectingClientChannel === reconnectPromise) {
        this.reconnectingClientChannel = undefined
      }
    })
    this.reconnectingClientChannel = reconnectPromise
    return reconnectPromise
  }

  // openClientChannelWithRetry retries transient connection-ack timeouts so
  // callers do not fail immediately while the runtime is still reconnecting.
  private async openClientChannelWithRetryImpl(): Promise<MessagePort> {
    const errors: Error[] = []
    for (const attempt of [0, 1, 2]) {
      try {
        return await this.openClientChannel()
      } catch (errAny) {
        const err = castToError(
          errAny,
          `WebRuntimeClient: ${this.clientId}: failed to connect to runtime`,
        )
        errors.push(err)
        if (attempt === 2) {
          break
        }
        await timeoutPromise(100)
      }
    }
    throw (
      errors[errors.length - 1] ??
      new Error(`WebRuntimeClient: ${this.clientId}: unable to connect to runtime`)
    )
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
    let err: Error | undefined
    if (!this.handleIncomingStream) {
      err = new Error(
        `${this.clientType.toString()}: handle stream: not implemented`,
      )
    } else {
      try {
        await this.handleIncomingStream(channel)
      } catch (e) {
        err = castToError(
          e,
          `${this.clientType.toString()}: handle stream: unknown error`,
        )
      }
    }
    if (err) {
      console.error(err.message)
      channel.close(err)
      return
    }
  }
}

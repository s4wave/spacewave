import { PacketStream, ChannelStream, castToError } from 'starpc'

import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import { ClientToWebRuntime, WebRuntimeToClient } from '../runtime/runtime.js'
import { timeoutPromise } from './timeout.js'
import { WebRuntimeClientChannelStreamOpts } from './web-runtime.js'

// OpenChannelFn opens the MessagePort to the WebRuntime.
type OpenChannelFn = (init: WebRuntimeClientInit) => Promise<MessagePort>

// HandleStreamFn handles an incoming RPC stream.
// Returns as soon as the stream has been passed off to be handled.
// Throws an error if we can't handle the incoming stream.
type HandleStreamFn = (ch: PacketStream) => Promise<void>

// HandleDisconnectedFn handles when the web runtime client was disconnected.
type HandleDisconnectedFn = (err?: Error) => Promise<void>

// WebRuntimeClient opens streams via a remote WebRuntime.
export class WebRuntimeClient {
  // clientChannel is the active message port to the remote.
  private clientChannel?: MessagePort

  constructor(
    public readonly webRuntimeId: string,
    public readonly clientId: string,
    public readonly clientType: WebRuntimeClientType,
    private openClientCh: OpenChannelFn,
    private handleIncomingStream: HandleStreamFn | null,
    private handleDisconnected: HandleDisconnectedFn | null,
  ) {}

  // waitConn opens and waits for the connection to be ready.
  public async waitConn() {
    await this.openClientChannel()
  }

  // openStream opens a RPC stream with the WebRuntimeHost.
  // the remote service depends on the WebRuntimeClientType.
  //
  // times out if the client does not ack within 3 seconds.
  public async openStream(): Promise<PacketStream> {
    // retry several times
    let err: Error | undefined
    for (let attempt = 0; attempt < 3; attempt++) {
      const clientPort = await this.openClientChannel()
      const streamChannel = new MessageChannel()
      const streamConn = new ChannelStream(
        this.clientId,
        streamChannel.port1,
        WebRuntimeClientChannelStreamOpts,
      )
      const msg = <ClientToWebRuntime>{
        from: this.clientId,
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

      console.log(`WebRuntimeClient: ${this.clientId}: opened stream with host`)
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
  private async openClientChannel(): Promise<MessagePort> {
    if (this.clientChannel) {
      return this.clientChannel
    }

    const init: WebRuntimeClientInit = {
      webRuntimeId: this.webRuntimeId,
      clientUuid: this.clientId,
      clientType: this.clientType,
    }
    const port = await this.openClientCh(init)
    port.onmessage = (ev) => {
      const data = ev.data
      if (typeof data !== 'object') {
        return
      }
      this.handleMessage(data, ev.ports)
    }
    this.clientChannel = port
    port.start()
    return port
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

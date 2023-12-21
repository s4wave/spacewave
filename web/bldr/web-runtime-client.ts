import { Stream } from 'starpc'

import {
  WebRuntimeClientInit,
  WebRuntimeClientType,
} from '../runtime/runtime.pb.js'
import { ClientToWebRuntime, WebRuntimeToClient } from '../runtime/runtime.js'
import { ChannelStream } from './channel.js'
import { timeoutPromise } from './timeout.js'
import { castToError } from 'starpc'

// OpenChannelFn opens the MessagePort to the WebRuntime.
type OpenChannelFn = (init: WebRuntimeClientInit) => Promise<MessagePort>

// HandleStreamFn handles an incoming RPC stream.
// Returns as soon as the stream has been passed off to be handled.
// Throws an error if we can't handle the incoming stream.
type HandleStreamFn = (ch: Stream) => Promise<void>

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
  ) {}

  // waitConn opens and waits for the connection to be ready.
  public async waitConn() {
    await this.openClientChannel()
  }

  // openStream opens a RPC stream with the WebRuntimeHost.
  // the remote service depends on the WebRuntimeClientType.
  //
  // times out if the client does not ack within 3 seconds.
  public async openStream(): Promise<Stream> {
    // retry several times
    let err: Error | undefined
    for (let attempt = 0; attempt < 3; attempt++) {
      const clientPort = await this.openClientChannel()
      const streamChannel = new MessageChannel()
      const streamConn = new ChannelStream<Uint8Array>(
        this.clientId,
        streamChannel.port1,
        false,
      )
      const msg = <ClientToWebRuntime>{
        from: this.clientId,
        openStream: true,
      }
      clientPort.postMessage(msg, [streamChannel.port2])
      await Promise.race([streamConn.waitRemoteOpen, timeoutPromise(1500)])
      if (!streamConn.isOpen) {
        streamConn.close()
        if (this.clientChannel === clientPort) {
          this.clientChannel.close()
          this.clientChannel = undefined
        }
        const msg = `WebRuntimeClient: ${this.clientId}: timeout opening stream with host`
        err = new Error(msg)
        console.warn(msg)
        // try again shortly.
        await timeoutPromise(100)
        continue
      }
      return streamConn
    }

    console.log(`WebRuntimeClient: ${this.clientId}: opened stream with host`)
    throw err || new Error('WebRuntimeClient: unable to open stream with host')
  }

  // close closes the client channel and signals the close to the remote.
  // note: the client can still be used again after calling close().
  public close() {
    if (this.clientChannel) {
      this.clientChannel.postMessage(<ClientToWebRuntime>{
        close: true,
      })
      this.clientChannel.close()
      this.clientChannel = undefined
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
    const channel = new ChannelStream<Uint8Array>(
      this.clientId,
      remoteMsgPort,
      true,
    )
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

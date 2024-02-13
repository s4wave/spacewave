import type { Sink, Source, Duplex } from 'it-stream-types'
import { pushable, Pushable } from 'it-pushable'

// ChannelStreamMessage is a message sent over the stream.
interface ChannelStreamMessage<T> {
  // from indicates who sent the message.
  from: string
  // ack indicates a remote joined the stream.
  ack?: boolean
  // opened indicates the remote has opened the stream.
  opened?: boolean
  // closed indicates the stream is closed.
  closed?: boolean
  // error indicates the stream has an error.
  error?: Error
  // data is any message data.
  data?: T
}

// Channel represents a channel we can open a stream over.
type Channel = MessagePort | { tx: BroadcastChannel; rx: BroadcastChannel }

// ChannelStream implements a Stream over a BroadcastChannel duplex or MessagePort.
export class ChannelStream<T>
  implements Duplex<AsyncGenerator<T>, Source<T>, Promise<void>>
{
  // channel is the read/write channel.
  public readonly channel: Channel
  // sink is the sink for incoming messages.
  public sink: Sink<Source<T>, Promise<void>>
  // source is the source for outgoing messages.
  public source: AsyncGenerator<T>
  // _source emits incoming data to the source.
  private readonly _source: {
    push: (val: T) => void
    end: (err?: Error) => void
  }
  // localId is the local identifier
  private readonly localId: string
  // localOpen indicates the local side has opened the stream.
  private localOpen: boolean
  // remoteOpen indicates the remote side has opened the stream.
  private remoteOpen: boolean
  // waitRemoteOpen indicates the remote side has opened the stream.
  public readonly waitRemoteOpen: Promise<void>
  // _remoteOpen fulfills the waitRemoteOpen promise.
  private _remoteOpen?: (err?: Error) => void
  // remoteAck indicates the remote side has acked the stream.
  private remoteAck: boolean
  // waitRemoteAck indicates the remote side has opened the stream.
  public readonly waitRemoteAck: Promise<void>
  // _remoteAck fulfills the waitRemoteAck promise.
  private _remoteAck?: (err?: Error) => void

  // isAcked checks if the stream is acknowledged by the remote.
  public get isAcked() {
    return this.remoteAck || false
  }

  // isOpen checks if the stream is opened by the remote.
  public get isOpen() {
    return this.remoteOpen || false
  }

  // remoteOpen indicates if we know the remote has already opened the stream.
  constructor(localId: string, channel: Channel, remoteOpen: boolean) {
    this.localId = localId
    this.channel = channel
    this.sink = this._createSink()

    this.localOpen = false
    this.remoteAck = remoteOpen
    this.remoteOpen = remoteOpen
    if (remoteOpen) {
      this.waitRemoteOpen = Promise.resolve()
      this.waitRemoteAck = Promise.resolve()
    } else {
      this.waitRemoteOpen = new Promise<void>((resolve, reject) => {
        this._remoteOpen = (err?: Error) => {
          if (err) {
            reject(err)
          } else {
            resolve()
          }
        }
      })
      this.waitRemoteOpen.catch(() => {})
      this.waitRemoteAck = new Promise<void>((resolve, reject) => {
        this._remoteAck = (err?: Error) => {
          if (err) {
            reject(err)
          } else {
            resolve()
          }
        }
      })
      this.waitRemoteAck.catch(() => {})
    }

    const source: Pushable<T> = pushable({ objectMode: true })
    this.source = source
    this._source = source

    const onMessage = this.onMessage.bind(this)
    if (channel instanceof MessagePort) {
      // MessagePort
      channel.onmessage = onMessage
      channel.start()
    } else {
      // BroadcastChannel
      channel.rx.onmessage = onMessage
    }
    this.postMessage({ ack: true })
  }

  // postMessage writes a message to the stream.
  private postMessage(msg: Partial<ChannelStreamMessage<T>>) {
    msg.from = this.localId
    if (this.channel instanceof MessagePort) {
      this.channel.postMessage(msg)
    } else {
      this.channel.tx.postMessage(msg)
    }
  }

  // close closes the broadcast channels.
  public close(error?: Error) {
    // write a message to indicate the stream is now closed.
    this.postMessage({ closed: true, error })
    // close channels
    if (this.channel instanceof MessagePort) {
      this.channel.close()
    } else {
      this.channel.tx.close()
      this.channel.rx.close()
    }
    if (!this.remoteOpen && this._remoteOpen) {
      this._remoteOpen(error || new Error('closed'))
    }
    if (!this.remoteAck && this._remoteAck) {
      this._remoteAck(error || new Error('closed'))
    }
  }

  // onLocalOpened indicates the local side has opened the read stream.
  private onLocalOpened() {
    if (!this.localOpen) {
      this.localOpen = true
      this.postMessage({ opened: true })
    }
  }

  // onRemoteAcked indicates the remote side has acked the stream.
  private onRemoteAcked() {
    if (!this.remoteAck) {
      this.remoteAck = true
      if (this._remoteAck) {
        this._remoteAck()
      }
    }
  }

  // onRemoteOpened indicates the remote side has opened the read stream.
  private onRemoteOpened() {
    if (!this.remoteOpen) {
      this.remoteOpen = true
      if (this._remoteOpen) {
        this._remoteOpen()
      }
    }
  }

  private _createSink(): Sink<Source<T>, Promise<void>> {
    return async (source: Source<T>) => {
      // make sure the remote is open before we send any data.
      await this.waitRemoteAck
      this.onLocalOpened()
      await this.waitRemoteOpen

      try {
        for await (const msg of source) {
          this.postMessage({ data: msg })
        }
        this.postMessage({ closed: true })
      } catch (error) {
        this.postMessage({ closed: true, error: error as Error })
      }
    }
  }

  private onMessage(ev: MessageEvent<ChannelStreamMessage<T>>) {
    const msg = ev.data
    if (!msg || msg.from === this.localId || !msg.from) {
      return
    }
    if (msg.ack || msg.opened) {
      this.onRemoteAcked()
    }
    if (msg.opened) {
      this.onRemoteOpened()
    }
    const { data, closed, error: err } = msg
    if (data) {
      this._source.push(data)
    }
    if (err) {
      this._source.end(err)
    } else if (closed) {
      this._source.end()
    }
  }
}

// newBroadcastChannelStream constructs a ChannelStream with a channel name.
export function newBroadcastChannelStream<T>(
  id: string,
  readName: string,
  writeName: string,
  remoteOpen: boolean,
): ChannelStream<T> {
  return new ChannelStream<T>(
    id,
    { tx: new BroadcastChannel(writeName), rx: new BroadcastChannel(readName) },
    remoteOpen,
  )
}

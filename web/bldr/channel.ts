// MessageHandler is the message handler callback.
export type MessageHandlerCallback = (msg: Uint8Array) => Promise<void>

// Channel implements a send and receive over BroadcastChannel.
export class Channel {
  // txCh is the channel to write to
  private txCh: BroadcastChannel
  // rxCh is the channel to read from
  private rxCh: BroadcastChannel

  constructor(
    // txID is the name of the channel to write to.
    private txID: string,
    // rxID is the name of the channel to read from.
    private rxID: string,
    // handler is the message handler
    private handler?: MessageHandlerCallback
  ) {
    this.txCh = new BroadcastChannel(txID)
    this.rxCh = new BroadcastChannel(rxID)
    this.rxCh.onmessage = async (ev) => {
      if (!(ev.data instanceof Uint8Array)) {
        console.log('drop non-uint8array message', ev.data)
        return
      }
      if (this.handler) {
        await this.handler(ev.data)
      }
    }
  }

  // getTxCh returns the tx channel.
  public getTxCh(): BroadcastChannel {
    return this.txCh
  }

  // getRxCh returns the rx channel.
  public getRxCh(): BroadcastChannel {
    return this.rxCh
  }

  // write writes a packet of data to the channel
  public write(data: Uint8Array) {
    this.txCh.postMessage(data)
  }

  // close closes the read and write channels.
  public close() {
    this.rxCh.close()
    this.txCh.close()
  }
}

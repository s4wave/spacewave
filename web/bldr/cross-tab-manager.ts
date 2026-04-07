// cross-tab-manager.ts manages brokered cross-tab MessagePort channels.
//
// Receives "direct-port" and "peer-gone" messages from the ServiceWorker
// cross-tab broker. Maintains a map of peerId -> MessagePort. Wraps ports
// as starpc ChannelStream for RPC compatibility.

import { ChannelStream, type PacketStream, type ChannelStreamOpts } from 'starpc'
import type { CrossTabBrokerMessage } from './cross-tab-broker.js'

// CrossTabChannelStreamOpts configures ChannelStreams for cross-tab channels.
// Higher timeouts than intra-tab since cross-tab involves more latency and
// background tabs may be throttled by the browser.
const CrossTabChannelStreamOpts: ChannelStreamOpts = {
  keepAliveMs: 15000,
  idleTimeoutMs: 90000,
}

// CrossTabManager receives brokered MessagePorts from the ServiceWorker
// and maintains direct channels to peer tabs.
export class CrossTabManager {
  // peers maps ServiceWorker client ID -> MessagePort.
  private peers = new Map<string, MessagePort>()

  constructor(private readonly localId: string) {}

  // handleMessage processes a cross-tab broker message from the ServiceWorker.
  // Returns true if the message was handled.
  handleMessage(data: unknown, ports: readonly MessagePort[]): boolean {
    if (typeof data !== 'object' || data === null) return false
    const msg = data as Record<string, unknown>
    if (msg.crossTab === 'direct-port') {
      const peerId = msg.peerId as string
      const port = ports[0]
      if (peerId && port) {
        this.addPeer(peerId, port)
      }
      return true
    }
    if (msg.crossTab === 'peer-gone') {
      const peerId = msg.peerId as string
      if (peerId) {
        this.removePeer(peerId)
      }
      return true
    }
    return false
  }

  // addPeer registers a brokered MessagePort for a peer tab.
  private addPeer(peerId: string, port: MessagePort) {
    const existing = this.peers.get(peerId)
    if (existing) {
      existing.close()
    }
    this.peers.set(peerId, port)
    port.start()
    console.log('cross-tab: channel to peer', peerId, 'established (' + this.peers.size + ' peers)')
  }

  // removePeer closes and removes the channel for a peer tab.
  private removePeer(peerId: string) {
    const port = this.peers.get(peerId)
    if (port) {
      port.close()
      this.peers.delete(peerId)
      console.log('cross-tab: peer', peerId, 'disconnected (' + this.peers.size + ' peers)')
    }
  }

  // openStream opens a StarPC-compatible ChannelStream to a peer tab.
  // Returns null if no channel exists for the peer.
  openStream(peerId: string): PacketStream | null {
    const port = this.peers.get(peerId)
    if (!port) return null

    // Create a sub-channel for this stream so the main port stays available.
    const { port1, port2 } = new MessageChannel()
    port.postMessage({ type: 'relay', port: port2 }, [port2])
    return new ChannelStream(this.localId, port1, CrossTabChannelStreamOpts)
  }

  // peerIds returns the list of connected peer tab IDs.
  get peerIds(): string[] {
    return [...this.peers.keys()]
  }

  // peerCount returns the number of connected peer tabs.
  get peerCount(): number {
    return this.peers.size
  }

  // close closes all peer channels.
  close() {
    for (const [, port] of this.peers) {
      port.close()
    }
    this.peers.clear()
  }
}

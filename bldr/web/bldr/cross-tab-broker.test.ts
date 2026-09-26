import { describe, expect, it, vi } from 'vitest'

import { handleCrossTabMessage } from './cross-tab-broker.js'

function createClient(id: string) {
  return { id, postMessage: vi.fn() }
}

function createClients(
  known: ReturnType<typeof createClient>[],
  windows: ReturnType<typeof createClient>[],
): Clients {
  return {
    get: vi.fn(async (id: string) => known.find((c) => c.id === id)),
    matchAll: vi.fn(async () => windows),
  } as unknown as Clients
}

describe('handleCrossTabMessage', () => {
  it('brokers a channel between the sender and each peer', async () => {
    const sender = createClient('sender')
    const peer = createClient('peer')
    await handleCrossTabMessage(
      createClients([sender, peer], [sender, peer]),
      'sender',
      { crossTab: 'hello' },
    )
    expect(peer.postMessage).toHaveBeenCalledTimes(1)
    expect(sender.postMessage).toHaveBeenCalledTimes(1)
    const peerPort = peer.postMessage.mock.calls[0][1][0] as MessagePort
    const senderPort = sender.postMessage.mock.calls[0][1][0] as MessagePort
    peerPort.close()
    senderPort.close()
  })

  it('does not send ports to peers when the sender is unknown', async () => {
    const peer = createClient('peer')
    await handleCrossTabMessage(createClients([peer], [peer]), 'sender', {
      crossTab: 'hello',
    })
    expect(peer.postMessage).not.toHaveBeenCalled()
  })
})

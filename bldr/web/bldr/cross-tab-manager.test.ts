import { describe, expect, it, vi } from 'vitest'

import { CrossTabManager } from './cross-tab-manager.js'

function createTestStream(localId: string, channel: MessagePort): never {
  return { localId, channel } as never
}

describe('CrossTabManager', () => {
  it('tracks brokered peer ports and removes gone peers', () => {
    const manager = new CrossTabManager('local')
    const channel = new MessageChannel()

    expect(
      manager.handleMessage({ crossTab: 'direct-port', peerId: 'peer-a' }, [
        channel.port1,
      ]),
    ).toBe(true)
    expect(manager.peerIds).toEqual(['peer-a'])
    expect(manager.peerCount).toBe(1)

    expect(
      manager.handleMessage({ crossTab: 'peer-gone', peerId: 'peer-a' }, []),
    ).toBe(true)
    expect(manager.peerIds).toEqual([])
  })

  it('forwards relay subchannels to the incoming stream handler', async () => {
    const handleIncomingStream = vi.fn().mockResolvedValue(undefined)
    const manager = new CrossTabManager(
      'local',
      handleIncomingStream,
      createTestStream,
    )
    const channel = new MessageChannel()

    manager.handleMessage({ crossTab: 'direct-port', peerId: 'peer-a' }, [
      channel.port1,
    ])

    const subChannel = new MessageChannel()
    channel.port2.postMessage({ type: 'relay' }, [subChannel.port1])

    await vi.waitFor(() =>
      expect(handleIncomingStream).toHaveBeenCalledTimes(1),
    )
  })

  it('creates relay subchannels on openStream', async () => {
    const manager = new CrossTabManager('local', undefined, createTestStream)
    const channel = new MessageChannel()
    const relays: MessagePort[] = []
    channel.port2.onmessage = (ev) => {
      if (ev.data?.type === 'relay' && ev.ports?.[0]) {
        relays.push(ev.ports[0])
      }
    }
    channel.port2.start()

    manager.handleMessage({ crossTab: 'direct-port', peerId: 'peer-a' }, [
      channel.port1,
    ])

    const stream = manager.openStream('peer-a')
    expect(stream).not.toBeNull()
    await vi.waitFor(() => expect(relays).toHaveLength(1))
  })
})

// @vitest-environment node
import { MessageChannel } from 'node:worker_threads'
import { expect, it } from 'vitest'

import { messagePortPacketStream } from './message-port-packet-stream.js'

it('exchanges raw Go packets and preserves responses after write EOF', async () => {
  const { port1, port2 } = new MessageChannel()
  const stream = messagePortPacketStream(port1 as unknown as MessagePort)
  const packets: unknown[] = []
  port2.on('message', (packet) => {
    packets.push(packet)
    if (packet === null) {
      port2.postMessage(new Uint8Array([7, 8]))
      port2.postMessage(null)
    }
  })

  try {
    await stream.sink([new Uint8Array(), new Uint8Array([1, 2])])
    expect(await stream.source.next()).toEqual({
      done: false,
      value: new Uint8Array([7, 8]),
    })
    expect(await stream.source.next()).toEqual({ done: true, value: undefined })
    expect(packets).toEqual([new Uint8Array(), new Uint8Array([1, 2]), null])
  } finally {
    await stream.close()
    port2.close()
  }
})

it.each(['close', 'abort'] as const)(
  '%s releases blocked reads and writes',
  async (action) => {
    const { port1, port2 } = new MessageChannel()
    const stream = messagePortPacketStream(port1 as unknown as MessagePort)
    const read = stream.source.next()
    const write = stream.sink(
      (async function* () {
        await new Promise(() => {})
        yield new Uint8Array()
      })(),
    )
    const result = Promise.allSettled([read, write])
    const error = new Error('cancelled')
    try {
      if (action === 'abort') stream.abort(error)
      else await stream.close()
      const settled = await result
      expect(settled.map((item) => item.status)).toEqual([
        action === 'abort' ? 'rejected' : 'fulfilled',
        action === 'abort' ? 'rejected' : 'fulfilled',
      ])
    } finally {
      await stream.close()
      port2.close()
    }
  },
)

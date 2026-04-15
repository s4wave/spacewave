import { describe, it, expect } from 'vitest'
import {
  SabBusEndpoint,
  SabBusStream,
  createBusSab,
  BROADCAST_ID,
} from './sab-bus.js'

const testOpts = { slotSize: 128, numSlots: 8 }

describe('SabBusEndpoint', () => {
  it('sends a message from A to B', async () => {
    const sab = createBusSab(testOpts)
    const a = new SabBusEndpoint(sab, 1, testOpts)
    const b = new SabBusEndpoint(sab, 2, testOpts)
    a.register()
    b.register()

    const msg = new Uint8Array([10, 20, 30])
    await a.write(2, msg)

    const received = await b.read()
    expect(received).not.toBeNull()
    expect(received!.sourceId).toBe(1)
    expect(received!.targetId).toBe(2)
    expect(received!.data).toEqual(msg)

    a.close()
    b.close()
  })

  it('filters messages not addressed to the reader', async () => {
    const sab = createBusSab(testOpts)
    const a = new SabBusEndpoint(sab, 1, testOpts)
    const b = new SabBusEndpoint(sab, 2, testOpts)
    const c = new SabBusEndpoint(sab, 3, testOpts)
    a.register()
    b.register()
    c.register()

    await a.write(3, new Uint8Array([1]))
    await a.write(2, new Uint8Array([2]))

    // B skips the first message (addressed to C) and gets the second.
    const bMsg = await b.read()
    expect(bMsg!.data).toEqual(new Uint8Array([2]))

    // C gets the first message.
    const cMsg = await c.read()
    expect(cMsg!.data).toEqual(new Uint8Array([1]))

    a.close()
    b.close()
    c.close()
  })

  it('delivers broadcast messages to all readers', async () => {
    const sab = createBusSab(testOpts)
    const a = new SabBusEndpoint(sab, 1, testOpts)
    const b = new SabBusEndpoint(sab, 2, testOpts)
    a.register()
    b.register()

    await a.write(BROADCAST_ID, new Uint8Array([99]))

    // Both endpoints should receive the broadcast.
    const aMsg = await a.read()
    expect(aMsg!.data).toEqual(new Uint8Array([99]))
    expect(aMsg!.targetId).toBe(BROADCAST_ID)

    const bMsg = await b.read()
    expect(bMsg!.data).toEqual(new Uint8Array([99]))

    a.close()
    b.close()
  })

  it('handles bidirectional exchange', async () => {
    const sab = createBusSab(testOpts)
    const a = new SabBusEndpoint(sab, 1, testOpts)
    const b = new SabBusEndpoint(sab, 2, testOpts)
    a.register()
    b.register()

    await a.write(2, new Uint8Array([11]))
    await b.write(1, new Uint8Array([22]))

    const bMsg = await b.read()
    expect(bMsg!.sourceId).toBe(1)
    expect(bMsg!.data).toEqual(new Uint8Array([11]))

    const aMsg = await a.read()
    expect(aMsg!.sourceId).toBe(2)
    expect(aMsg!.data).toEqual(new Uint8Array([22]))

    a.close()
    b.close()
  })

  it('returns null when bus is closed', async () => {
    const sab = createBusSab(testOpts)
    const a = new SabBusEndpoint(sab, 1, testOpts)
    a.register()

    a.closeAll()

    const msg = await a.read()
    expect(msg).toBeNull()
  })

  it('wraps around the ring buffer', async () => {
    const sab = createBusSab(testOpts)
    const a = new SabBusEndpoint(sab, 1, testOpts)
    const b = new SabBusEndpoint(sab, 2, testOpts)
    // Only register B as a reader. A only writes, so it does not
    // need a reader slot and will not hold up the ring.
    b.register()

    // 8 slots, send 12 messages to exercise wrapping.
    for (let i = 0; i < 12; i++) {
      await a.write(2, new Uint8Array([i]))
      const msg = await b.read()
      expect(msg!.data).toEqual(new Uint8Array([i]))
    }

    a.close()
    b.close()
  })

  it('reuses reader slots after endpoints close', () => {
    const sab = createBusSab(testOpts)

    for (const i of Array.from({ length: 32 }, (_, idx) => idx)) {
      const ep = new SabBusEndpoint(sab, i + 1, testOpts)
      ep.register()
      ep.close()
    }
  })

  it('rejects more than 16 simultaneous readers', () => {
    const sab = createBusSab(testOpts)
    const eps = Array.from(
      { length: 16 },
      (_, i) => new SabBusEndpoint(sab, i + 1, testOpts),
    )
    for (const ep of eps) {
      ep.register()
    }

    const extra = new SabBusEndpoint(sab, 17, testOpts)
    expect(() => extra.register()).toThrow('SabBus: max readers (16) exceeded')

    for (const ep of eps) {
      ep.close()
    }
  })
})

describe('SabBusStream', () => {
  it('adapts bus endpoint to PacketStream interface', async () => {
    const sab = createBusSab(testOpts)
    const epA = new SabBusEndpoint(sab, 1, testOpts)
    const epB = new SabBusEndpoint(sab, 2, testOpts)
    epA.register()
    epB.register()

    // Create streams: A talks to B (targetId=2), B talks to A (targetId=1).
    const streamA = new SabBusStream(epA, 2)
    const streamB = new SabBusStream(epB, 1)

    const msg = new Uint8Array([42, 43, 44])

    // A writes via sink.
    const doneA = streamA.sink(
      (async function* () {
        yield msg
      })(),
    )

    // B reads via source.
    const result = await streamB.source.next()
    expect(result.done).toBe(false)
    expect(result.value).toEqual(msg)

    streamA.close()
    streamB.close()
    epA.close()
    epB.close()
    await doneA
  })
})

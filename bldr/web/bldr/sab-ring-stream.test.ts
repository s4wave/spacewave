import { describe, it, expect } from 'vitest'
import { SabRingStream, createSabPair } from './sab-ring-stream.js'

// Small ring for tests: 128-byte slots, 4 slots.
const testOpts = { slotSize: 128, numSlots: 4 }

function makePair() {
  const { aSab, bSab } = createSabPair(testOpts)
  const a = new SabRingStream(aSab, bSab, testOpts)
  const b = new SabRingStream(bSab, aSab, testOpts)
  return { a, b }
}

describe('SabRingStream', () => {
  it('sends and receives a single message', async () => {
    const { a, b } = makePair()
    const msg = new Uint8Array([1, 2, 3, 4, 5])

    const done = a.sink(
      (async function* () {
        yield msg
      })(),
    )

    const result = await b.source.next()
    expect(result.done).toBe(false)
    expect(result.value).toEqual(msg)

    a.close()
    b.close()
    await done
  })

  it('sends multiple messages in order', async () => {
    const { a, b } = makePair()
    const messages = [
      new Uint8Array([10, 20]),
      new Uint8Array([30, 40, 50]),
      new Uint8Array([60]),
    ]

    const done = a.sink(
      (async function* () {
        for (const m of messages) yield m
      })(),
    )

    for (const expected of messages) {
      const result = await b.source.next()
      expect(result.done).toBe(false)
      expect(result.value).toEqual(expected)
    }

    a.close()
    b.close()
    await done
  })

  it('propagates close to remote reader', async () => {
    const { a, b } = makePair()

    // Close the tx side via sink finishing with no messages.
    const done = a.sink(
      (async function* () {
        // yield nothing
      })(),
    )
    await done

    // B should see the stream end.
    const result = await b.source.next()
    expect(result.done).toBe(true)

    b.close()
  })

  it('handles bidirectional communication', async () => {
    const { a, b } = makePair()
    const msgAB = new Uint8Array([11, 22, 33])
    const msgBA = new Uint8Array([44, 55, 66])

    const doneA = a.sink(
      (async function* () {
        yield msgAB
      })(),
    )
    const doneB = b.sink(
      (async function* () {
        yield msgBA
      })(),
    )

    const fromA = await b.source.next()
    expect(fromA.value).toEqual(msgAB)

    const fromB = await a.source.next()
    expect(fromB.value).toEqual(msgBA)

    a.close()
    b.close()
    await Promise.all([doneA, doneB])
  })

  it('handles zero-length messages', async () => {
    const { a, b } = makePair()
    const empty = new Uint8Array(0)

    const done = a.sink(
      (async function* () {
        yield empty
      })(),
    )

    const result = await b.source.next()
    expect(result.done).toBe(false)
    expect(result.value).toEqual(empty)

    a.close()
    b.close()
    await done
  })

  it('fills and drains the ring buffer', async () => {
    const { a, b } = makePair()
    // 4 slots, send 8 messages to exercise wrap-around.
    const count = 8
    const messages: Uint8Array[] = []
    for (let i = 0; i < count; i++) {
      messages.push(new Uint8Array([i]))
    }

    const done = a.sink(
      (async function* () {
        for (const m of messages) yield m
      })(),
    )

    for (let i = 0; i < count; i++) {
      const result = await b.source.next()
      expect(result.done).toBe(false)
      expect(result.value).toEqual(new Uint8Array([i]))
    }

    a.close()
    b.close()
    await done
  })

  it('rejects messages exceeding slot size', async () => {
    const { a, b } = makePair()
    // Max payload = 128 - 4 = 124 bytes. Send 125.
    const oversized = new Uint8Array(125)

    const done = a.sink(
      (async function* () {
        yield oversized
      })(),
    )

    // Sink should catch the error and close.
    await done
    // Stream should be closed after error.
    expect(await b.source.next()).toMatchObject({ done: true })
  })
})

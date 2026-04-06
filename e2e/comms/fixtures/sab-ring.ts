// sab-ring.ts - SAB ring buffer point-to-point test fixture.
//
// Creates a SabRingStream pair, sends messages, verifies ordering,
// tests bidirectional and close propagation.

import {
  SabRingStream,
  createSabPair,
} from '../../../web/bldr/sab-ring-stream.js'

declare global {
  interface Window {
    __results: {
      pass: boolean
      detail: string
      sendRecv: boolean
      bidirectional: boolean
      close: boolean
      messageCount: number
    }
  }
}

async function collectN(
  source: AsyncIterable<Uint8Array>,
  n: number,
  timeoutMs: number,
): Promise<Uint8Array[]> {
  const msgs: Uint8Array[] = []
  const deadline = Date.now() + timeoutMs
  for await (const chunk of source) {
    msgs.push(new Uint8Array(chunk))
    if (msgs.length >= n) break
    if (Date.now() > deadline) break
  }
  return msgs
}

async function run() {
  const log = document.getElementById('log')!
  const errors: string[] = []

  try {
    // Test 1: Send 10 messages A->B, verify all received in order.
    // Use enough slots that the ring does not fill before the reader starts.
    const opts = { slotSize: 256, numSlots: 32 }
    let sendRecv = false
    {
      const { aSab, bSab } = createSabPair(opts)
      const streamA = new SabRingStream(aSab, bSab, opts)
      const streamB = new SabRingStream(bSab, aSab, opts)

      const count = 10
      const recvPromise = collectN(streamB.source, count, 5000)

      for (let i = 0; i < count; i++) {
        const data = new Uint8Array([i])
        await streamA.sink(
          (async function* () {
            yield data
          })(),
        )
      }

      const received = await recvPromise
      if (received.length !== count) {
        errors.push(`sendRecv: got ${received.length} msgs, want ${count}`)
      } else {
        let ok = true
        for (let i = 0; i < count; i++) {
          if (received[i][0] !== i) {
            errors.push(`sendRecv: msg[${i}]=${received[i][0]}, want ${i}`)
            ok = false
            break
          }
        }
        sendRecv = ok
      }

      streamA.close()
      streamB.close()
    }

    // Test 2: Bidirectional - both sides send simultaneously.
    let bidirectional = false
    {
      const { aSab, bSab } = createSabPair(opts)
      const streamA = new SabRingStream(aSab, bSab, opts)
      const streamB = new SabRingStream(bSab, aSab, opts)

      const count = 5
      const recvA = collectN(streamA.source, count, 5000)
      const recvB = collectN(streamB.source, count, 5000)

      // A sends 0xAA bytes, B sends 0xBB bytes.
      for (let i = 0; i < count; i++) {
        await streamA.sink(
          (async function* () {
            yield new Uint8Array([0xaa, i])
          })(),
        )
        await streamB.sink(
          (async function* () {
            yield new Uint8Array([0xbb, i])
          })(),
        )
      }

      const msgsA = await recvA
      const msgsB = await recvB

      if (msgsA.length === count && msgsB.length === count) {
        let ok = true
        for (let i = 0; i < count; i++) {
          if (msgsB[i][0] !== 0xaa || msgsB[i][1] !== i) {
            errors.push(`bidir: B got wrong msg at ${i}`)
            ok = false
            break
          }
          if (msgsA[i][0] !== 0xbb || msgsA[i][1] !== i) {
            errors.push(`bidir: A got wrong msg at ${i}`)
            ok = false
            break
          }
        }
        bidirectional = ok
      } else {
        errors.push(
          `bidir: A got ${msgsA.length}, B got ${msgsB.length}, want ${count}`,
        )
      }

      streamA.close()
      streamB.close()
    }

    // Test 3: Close propagation - closing A's sink should end B's source.
    let closeOk = false
    {
      const { aSab, bSab } = createSabPair(opts)
      const streamA = new SabRingStream(aSab, bSab, opts)
      const streamB = new SabRingStream(bSab, aSab, opts)

      // Send one message then close.
      await streamA.sink(
        (async function* () {
          yield new Uint8Array([42])
        })(),
      )
      streamA.close()

      // B should receive the message and then the source should end.
      const msgs: Uint8Array[] = []
      const deadline = Date.now() + 3000
      for await (const chunk of streamB.source) {
        msgs.push(new Uint8Array(chunk))
        if (Date.now() > deadline) break
      }

      if (msgs.length >= 1 && msgs[0][0] === 42) {
        closeOk = true
      } else {
        errors.push(`close: got ${msgs.length} msgs, first=${msgs[0]?.[0]}`)
      }

      streamB.close()
    }

    const pass = sendRecv && bidirectional && closeOk && errors.length === 0
    window.__results = {
      pass,
      detail: errors.length > 0 ? errors.join('; ') : 'all tests passed',
      sendRecv,
      bidirectional,
      close: closeOk,
      messageCount: 10,
    }
  } catch (err) {
    window.__results = {
      pass: false,
      detail: `error: ${err}`,
      sendRecv: false,
      bidirectional: false,
      close: false,
      messageCount: 0,
    }
  }

  log.textContent = 'DONE'
}

run()
